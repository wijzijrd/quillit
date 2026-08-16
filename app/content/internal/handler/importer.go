package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/quillit/contentengine/parse"

	"github.com/quillit/content-svc/internal/authz"
)

const maxImportBody = 50 << 20 // spec §2

type ImportHandler struct {
	db        *sql.DB
	jwtSecret []byte
	blobs     BlobStore
	checker   authz.Checker
}

func NewImport(db *sql.DB, jwtSecret string, blobs BlobStore, checker authz.Checker) *ImportHandler {
	return &ImportHandler{db: db, jwtSecret: []byte(jwtSecret), blobs: blobs, checker: checker}
}

type ImportReportRow struct {
	Path   string `json:"path"`
	Action string `json:"action"` // create|overwrite|skip|conflict|error
	Detail string `json:"detail,omitempty"`
}

type ImportFacetsReport struct {
	Created []string `json:"created"`
}

type ImportImageRow struct {
	Path     string `json:"path"`
	Uploaded bool   `json:"uploaded"`
	Detail   string `json:"detail,omitempty"`
}

type ImportResponse struct {
	Applied bool               `json:"applied"`
	Report  []ImportReportRow  `json:"report"`
	Facets  ImportFacetsReport `json:"facets"`
	Images  []ImportImageRow   `json:"images"`
}

type importEntryError struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

var kebabPath = regexp.MustCompile(`^[a-z0-9-]+$`)

// validImportPath checks slug and every directory segment against the DB's
// kebab-case CHECK constraint — we reject with a message rather than let
// the INSERT fail opaquely. No silent slugification (spec §2).
func validImportPath(item importItem) error {
	if !kebabPath.MatchString(item.Slug) {
		return fmt.Errorf("folder name %q is not a valid slug (lowercase letters, digits, hyphens only)", item.Slug)
	}
	if item.DirectoryPath != "" {
		for _, seg := range strings.Split(item.DirectoryPath, "/") {
			if !kebabPath.MatchString(seg) {
				return fmt.Errorf("directory segment %q is not valid (lowercase letters, digits, hyphens only)", seg)
			}
		}
	}
	return nil
}

func entryPathOf(dir, slug string) string {
	if dir == "" {
		return slug
	}
	return dir + "/" + slug
}

// ImportProject godoc
// @Summary      Import a CLI project tarball into a project
// @Description  Body is a tar.gz of a CLI project directory. Dry-run by default; validates every entry with the shared content-engine parser, applies atomically. See docs/superpowers/specs/2026-08-16-cli-project-import-design.md.
// @Tags         entries
// @Accept       application/gzip
// @Produce      json
// @Param        id            path   string  true   "Project ID"
// @Param        mode          query  string  false  "dry-run (default) or apply"
// @Param        onConflict    query  string  false  "fail (default), skip, overwrite, or suffix"
// @Param        createFacets  query  string  false  "true to add missing facets to the project vocabulary"
// @Success      200  {object}  ImportResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      422  {object}  map[string]any
// @Router       /content/projects/{id}/import [post]
func (h *ImportHandler) ImportProject(w http.ResponseWriter, r *http.Request) {
	if h.blobs == nil {
		writeError(w, http.StatusServiceUnavailable, "blob storage not configured")
		return
	}
	projectID := chi.URLParam(r, "id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "missing project id")
		return
	}
	ownerID, ok := requireProjectMember(w, r, h.jwtSecret, h.checker, projectID)
	if !ok {
		return
	}

	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "dry-run"
	}
	if mode != "dry-run" && mode != "apply" {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid mode %q: want dry-run or apply", mode))
		return
	}
	onConflict := r.URL.Query().Get("onConflict")
	if onConflict == "" {
		onConflict = "fail"
	}
	switch onConflict {
	case "fail", "skip", "overwrite", "suffix":
	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid onConflict %q: want fail, skip, overwrite, or suffix", onConflict))
		return
	}
	createFacets := r.URL.Query().Get("createFacets") == "true"

	items, err := readImportTarball(http.MaxBytesReader(w, r.Body, maxImportBody))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(items) == 0 {
		writeError(w, http.StatusBadRequest, "tarball contains no entries")
		return
	}

	// ── Validation phase: collect EVERY failure before reporting (fail loud, once).
	var entryErrs []importEntryError
	parsedItems := make([]*parse.Entry, len(items))
	for i, item := range items {
		p := entryPathOf(item.DirectoryPath, item.Slug)
		if err := validImportPath(item); err != nil {
			entryErrs = append(entryErrs, importEntryError{Path: p, Error: err.Error()})
			continue
		}
		parsed, err := parse.Parse(item.Body)
		if err != nil {
			entryErrs = append(entryErrs, importEntryError{Path: p, Error: err.Error()})
			continue
		}
		parsedItems[i] = parsed
	}

	vocab, sorted, err := effectiveFacetVocabulary(r.Context(), h.db, projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	missingSet := map[string]bool{}
	for i, item := range items {
		if parsedItems[i] == nil {
			continue
		}
		if err := validateFacets(parsedItems[i], vocab, sorted); err != nil {
			var ufe UnknownFacetError
			if errors.As(err, &ufe) {
				missingSet[ufe.Facet] = true
				// Re-validate against an augmented vocabulary to find ALL
				// missing facets in this entry, not just the first.
				aug := map[string]bool{}
				for k := range vocab {
					aug[k] = true
				}
				for {
					aug[ufe.Facet] = true
					if err := validateFacets(parsedItems[i], aug, sorted); err == nil {
						break
					} else if !errors.As(err, &ufe) {
						entryErrs = append(entryErrs, importEntryError{Path: entryPathOf(item.DirectoryPath, item.Slug), Error: err.Error()})
						break
					}
					missingSet[ufe.Facet] = true
				}
			} else {
				entryErrs = append(entryErrs, importEntryError{Path: entryPathOf(item.DirectoryPath, item.Slug), Error: err.Error()})
			}
		}
	}
	missing := make([]string, 0, len(missingSet))
	for f := range missingSet {
		missing = append(missing, f)
	}
	sort.Strings(missing)

	if len(entryErrs) > 0 || (len(missing) > 0 && !createFacets) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":         "validation failed",
			"entries":       entryErrs,
			"missingFacets": missing,
		})
		return
	}

	// ── Transactional phase. Blob writes are deferred until after the
	// commit decision so dry-run leaves zero traces (spec §4.7).
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer tx.Rollback()

	resp := ImportResponse{Report: []ImportReportRow{}, Facets: ImportFacetsReport{Created: []string{}}, Images: []ImportImageRow{}}

	if createFacets {
		for _, f := range missing {
			if _, err := tx.ExecContext(r.Context(),
				`INSERT OR IGNORE INTO project_facets (project_id, name) VALUES (?, ?)`, projectID, f); err != nil {
				writeError(w, http.StatusInternalServerError, "db error")
				return
			}
			resp.Facets.Created = append(resp.Facets.Created, f)
		}
	}

	type blobWrite struct {
		key, contentType string
		data             []byte
	}
	var blobWrites []blobWrite

	now := nowUnix()
	conflictFail := false
	type applied struct {
		id     string
		parsed *parse.Entry
		item   importItem
	}
	var appliedEntries []applied

	for i, item := range items {
		p := entryPathOf(item.DirectoryPath, item.Slug)
		slug := item.Slug

		var existingID string
		err := tx.QueryRowContext(r.Context(),
			`SELECT id FROM entries WHERE project_id = ? AND directory_path = ? AND slug = ?`,
			projectID, item.DirectoryPath, slug).Scan(&existingID)
		if err != nil && err != sql.ErrNoRows {
			writeError(w, http.StatusInternalServerError, "db error")
			return
		}
		exists := err == nil

		action := "create"
		switch {
		case exists && onConflict == "fail":
			resp.Report = append(resp.Report, ImportReportRow{Path: p, Action: "conflict", Detail: "an entry already exists at this path"})
			conflictFail = true
			continue
		case exists && onConflict == "skip":
			resp.Report = append(resp.Report, ImportReportRow{Path: p, Action: "skip", Detail: "an entry already exists at this path"})
			continue
		case exists && onConflict == "overwrite":
			action = "overwrite"
		case exists && onConflict == "suffix":
			for n := 2; ; n++ {
				candidate := fmt.Sprintf("%s-%d", item.Slug, n)
				var one int
				err := tx.QueryRowContext(r.Context(),
					`SELECT 1 FROM entries WHERE project_id = ? AND directory_path = ? AND slug = ?`,
					projectID, item.DirectoryPath, candidate).Scan(&one)
				if err == sql.ErrNoRows {
					slug = candidate
					break
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, "db error")
					return
				}
			}
			p = entryPathOf(item.DirectoryPath, slug)
		}

		parsed := parsedItems[i]
		title := parsed.Frontmatter.Name
		if title == "" {
			title = slug
		}
		tagsJSON := marshalTags(parsed.Frontmatter.Tags)

		var id string
		if action == "overwrite" {
			id = existingID
			if _, err := tx.ExecContext(r.Context(),
				`UPDATE entries SET title = ?, tags = ?, updated_at = ? WHERE id = ?`,
				title, tagsJSON, now, id); err != nil {
				writeError(w, http.StatusInternalServerError, "db error")
				return
			}
		} else {
			id = newID()
			if _, err := tx.ExecContext(r.Context(), `
				INSERT INTO entries (id, project_id, slug, directory_path, title, tags, owner_user_id, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, id, projectID, slug, item.DirectoryPath, title, tagsJSON, nullStr(ownerID), now, now); err != nil {
				writeError(w, http.StatusInternalServerError, "db error")
				return
			}
		}

		blobWrites = append(blobWrites, blobWrite{key: bodyKey(id), contentType: "text/markdown; charset=utf-8", data: item.Body})
		for _, asset := range item.Assets {
			ct := mime.TypeByExtension(path.Ext(asset.Name))
			if !isAllowedImageType(ct) {
				resp.Images = append(resp.Images, ImportImageRow{Path: entryPathOf(item.DirectoryPath, item.Slug) + "/" + asset.Name, Uploaded: false, Detail: "not an allowed image type"})
				continue
			}
			blobWrites = append(blobWrites, blobWrite{
				key:         fmt.Sprintf("entries/%s/images/%s", id, asset.Name),
				contentType: ct,
				data:        asset.Data,
			})
			resp.Images = append(resp.Images, ImportImageRow{Path: entryPathOf(item.DirectoryPath, item.Slug) + "/" + asset.Name, Uploaded: mode == "apply"})
		}

		resp.Report = append(resp.Report, ImportReportRow{Path: p, Action: action})
		appliedEntries = append(appliedEntries, applied{id: id, parsed: parsed, item: item})
	}

	if conflictFail {
		// onConflict=fail with any conflict: nothing is applied, in either
		// mode. Roll back (deferred) and return the report.
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// ── Second pass: every imported entry's links compile AFTER all rows
	// exist, so in-batch wikilinks resolve to real ids (spec §4.5, AC 4).
	for _, a := range appliedEntries {
		if err := recompileLinks(r.Context(), tx, a.id, projectID, a.parsed); err != nil {
			writeError(w, http.StatusInternalServerError, "db error")
			return
		}
	}

	// Pre-existing dangling links whose target now exists get re-resolved.
	if err := reresolveDanglingLinks(r.Context(), tx, projectID); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	if mode == "dry-run" {
		// Deferred tx.Rollback undoes everything; no blobs were written.
		writeJSON(w, http.StatusOK, resp)
		return
	}

	for _, bw := range blobWrites {
		if err := h.blobs.Put(r.Context(), bw.key, bw.contentType, bw.data); err != nil {
			for _, undo := range blobWrites {
				_ = h.blobs.Delete(r.Context(), undo.key)
			}
			writeError(w, http.StatusInternalServerError, "blob error")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		for _, undo := range blobWrites {
			_ = h.blobs.Delete(r.Context(), undo.key)
		}
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	resp.Applied = true
	writeJSON(w, http.StatusOK, resp)
}

// reresolveDanglingLinks re-resolves unresolved entry_links in projectID —
// after an import, targets that were dangling may now exist (spec §4.5).
func reresolveDanglingLinks(ctx context.Context, tx *sql.Tx, projectID string) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT el.rowid, el.target_path FROM entry_links el
		JOIN entries e ON e.id = el.entry_id
		WHERE el.resolved = 0 AND e.project_id = ?
	`, projectID)
	if err != nil {
		return err
	}
	type dangling struct {
		rowid int64
		path  string
	}
	var links []dangling
	for rows.Next() {
		var d dangling
		if err := rows.Scan(&d.rowid, &d.path); err != nil {
			rows.Close()
			return err
		}
		links = append(links, d)
	}
	rows.Close()
	for _, d := range links {
		id, resolved, err := resolveEntryPath(ctx, tx, projectID, d.path)
		if err != nil {
			return err
		}
		if resolved {
			if _, err := tx.ExecContext(ctx,
				`UPDATE entry_links SET target_entry_id = ?, resolved = 1 WHERE rowid = ?`, id, d.rowid); err != nil {
				return err
			}
		}
	}
	return nil
}

func marshalTags(tags []string) string {
	b, _ := json.Marshal(tags)
	return string(b)
}
