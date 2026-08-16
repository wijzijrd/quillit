package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/quillit/contentengine/parse"

	"github.com/quillit/content-svc/internal/authz"
)

// SearchHandler serves #43's project-scoped full-text search, backed by the
// entries_fts FTS5 virtual table (internal/db/db.go toV3). The same handler
// also backs the "[[" wikilink-autocomplete lookup (mode=lookup) — one query
// implementation, per docs/web-refactor-spec.md §6.4's "Decide whether this
// is the same endpoint with a mode=lookup style param or a separate
// lightweight endpoint — whichever avoids duplicating the underlying query
// logic."
type SearchHandler struct {
	db        *sql.DB
	jwtSecret []byte
	checker   authz.Checker
}

func NewSearch(db *sql.DB, jwtSecret string, checker authz.Checker) *SearchHandler {
	return &SearchHandler{db: db, jwtSecret: []byte(jwtSecret), checker: checker}
}

// SearchResult is one hit — full-text or lookup — with enough fields for a
// caller to construct a "[[directory_path/slug|title]]" wikilink without a
// follow-up request.
type SearchResult struct {
	ID            string   `json:"id"`
	ProjectID     string   `json:"projectId"`
	Slug          string   `json:"slug"`
	DirectoryPath string   `json:"directoryPath"`
	Title         string   `json:"title"`
	Tags          []string `json:"tags"`
}

const maxSearchResults = 50
const maxLookupResults = 20

// Search godoc
// @Summary      Full-text search over a project's entries, or wikilink-autocomplete lookup
// @Description  Default mode ranks matches (bm25) across title/tags/body. mode=lookup instead does a prefix/substring match on title only, for the "[[" editor autocomplete.
// @Tags         search
// @Produce      json
// @Param        id    path   string  true   "Project ID"
// @Param        q     query  string  true   "Search query"
// @Param        mode  query  string  false  "search (default) or lookup"
// @Success      200   {array}  SearchResult
// @Failure      400   {object}  ErrorResponse
// @Router       /content/projects/{id}/search [get]
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "missing project id")
		return
	}
	if _, ok := requireProjectMember(w, r, h.jwtSecret, h.checker, projectID); !ok {
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, []SearchResult{})
		return
	}

	var rows *sql.Rows
	var err error
	if r.URL.Query().Get("mode") == "lookup" {
		rows, err = h.db.QueryContext(r.Context(), `
			SELECT id, project_id, slug, directory_path, title, tags
			FROM entries
			WHERE project_id = ? AND title LIKE ? ESCAPE '\'
			ORDER BY title
			LIMIT ?
		`, projectID, likePattern(q), maxLookupResults)
	} else {
		rows, err = h.db.QueryContext(r.Context(), `
			SELECT e.id, e.project_id, e.slug, e.directory_path, e.title, e.tags
			FROM entries_fts
			JOIN entries e ON e.id = entries_fts.entry_id
			WHERE entries_fts MATCH ? AND e.project_id = ?
			ORDER BY bm25(entries_fts)
			LIMIT ?
		`, ftsMatchQuery(q), projectID, maxSearchResults)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	results := []SearchResult{}
	for rows.Next() {
		res, err := scanSearchResult(rows)
		if err != nil {
			continue
		}
		results = append(results, res)
	}
	writeJSON(w, http.StatusOK, results)
}

func scanSearchResult(rows *sql.Rows) (SearchResult, error) {
	var res SearchResult
	var tagsJSON string
	if err := rows.Scan(&res.ID, &res.ProjectID, &res.Slug, &res.DirectoryPath, &res.Title, &tagsJSON); err != nil {
		return res, err
	}
	_ = json.Unmarshal([]byte(tagsJSON), &res.Tags)
	return res, nil
}

// likePattern builds a substring-match LIKE pattern from q, escaping LIKE's
// own wildcard characters ("%", "_") so they're matched literally rather
// than as wildcards when they happen to appear in a title query.
func likePattern(q string) string {
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
	return "%" + esc + "%"
}

// ftsMatchQuery turns free-form user input q into a safe FTS5 MATCH query:
// wrapped as a quoted phrase so punctuation/operators in q (which FTS5's
// own query syntax would otherwise try to parse — and could reject with a
// syntax error) are always treated as literal text to search for, never as
// FTS5 query syntax.
func ftsMatchQuery(q string) string {
	return `"` + strings.ReplaceAll(q, `"`, `""`) + `"`
}

// refreshSearchIndex replaces entryID's row in entries_fts with content
// derived from title, tags, and parsed's full body — the search analogue of
// recompileLinks's entry_links refresh (links.go): a save is the staleness
// event, so every write recomputes the full row rather than diffing.
//
// Called only when the body actually changed (same condition entries.go
// already gates recompileLinks on): title/tags/body are the only columns
// this index stores, and none of them can change without a body write.
func refreshSearchIndex(ctx context.Context, tx *sql.Tx, entryID, title string, tags []string, parsed *parse.Entry) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM entries_fts WHERE entry_id = ?`, entryID); err != nil {
		return err
	}
	body := plainText(parsed)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO entries_fts (entry_id, title, tags, body)
		VALUES (?, ?, ?, ?)
	`, entryID, title, strings.Join(tags, " "), body)
	return err
}

// deleteSearchIndex removes entryID's entries_fts row. entries_fts is a
// virtual table with no foreign key to entries, so — unlike entry_links,
// which cascades via ON DELETE CASCADE — this needs an explicit call
// alongside the entries row delete (entries.go Delete, which itself runs
// outside a transaction, so this matches that non-transactional shape).
func deleteSearchIndex(ctx context.Context, db *sql.DB, entryID string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM entries_fts WHERE entry_id = ?`, entryID)
	return err
}

// plainText flattens a parsed entry's body into prose suitable for indexing:
// every block's own Content, in document order (parse.Parse already
// separates ":::secret"/":::card <facet>"/closing ":::" fence lines out as
// block structure rather than Content — see pkg/contentengine/parse's Block
// doc — so those markers are never present here to begin with), with each
// block's raw "[[path|label]]" wikilink syntax collapsed to its label and
// common inline/block markdown punctuation stripped, so search matches the
// words a person would actually read rather than markup noise.
//
// pkg/contentengine/parse offers no ready-made plain-text extraction (it
// stops at structured Blocks — rendering markdown is render's job, and that
// package produces HTML, not prose), so this is a small purpose-built
// extractor rather than a reused one.
func plainText(entry *parse.Entry) string {
	var b strings.Builder
	var walk func(blocks []parse.Block)
	walk = func(blocks []parse.Block) {
		for _, blk := range blocks {
			if blk.Content != "" {
				b.WriteString(stripMarkup(blk.Content))
				b.WriteString(" ")
			}
			if len(blk.Blocks) > 0 {
				walk(blk.Blocks)
			}
		}
	}
	walk(entry.Body)
	return strings.TrimSpace(b.String())
}

var (
	mdHeaderRe     = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	mdBlockquoteRe = regexp.MustCompile(`(?m)^>\s?`)
	mdBulletRe     = regexp.MustCompile(`(?m)^\s*[-*+]\s+`)
	mdOrderedRe    = regexp.MustCompile(`(?m)^\s*\d+\.\s+`)
	mdEmphasisRe   = regexp.MustCompile("(\\*\\*|__|\\*|`)")
	mdImageRe      = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	mdLinkRe       = regexp.MustCompile(`\[([^\]]+)\]\(([^)]*)\)`)
)

// stripMarkup strips wikilink and common markdown syntax from one block's
// raw Content, leaving prose. It's deliberately not a full markdown parser
// — just enough to keep structural punctuation out of the search index.
func stripMarkup(content string) string {
	// Wikilinks first: replace each raw "[[path|label]]"/"[[path]]"
	// occurrence with its label — mirrors render.renderMarkdown's
	// placeholder substitution, but the end result here is plain text
	// rather than an <a> tag.
	for _, link := range parse.ExtractLinks(content) {
		raw := "[[" + link.Path
		if link.Label != link.Path {
			raw += "|" + link.Label
		}
		raw += "]]"
		content = strings.Replace(content, raw, link.Label, 1)
	}

	content = mdImageRe.ReplaceAllString(content, "$1")
	content = mdLinkRe.ReplaceAllString(content, "$1")
	content = mdHeaderRe.ReplaceAllString(content, "")
	content = mdBlockquoteRe.ReplaceAllString(content, "")
	content = mdBulletRe.ReplaceAllString(content, "")
	content = mdOrderedRe.ReplaceAllString(content, "")
	content = mdEmphasisRe.ReplaceAllString(content, "")
	return content
}
