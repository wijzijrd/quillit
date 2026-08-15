package handler

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/quillit/contentengine/parse"
)

// LinksHandler serves the HTTP-facing endpoints for entry_links — reading a
// single entry's compiled outgoing links, and recompiling an entire
// project's link index in bulk. Both endpoints are thin wrappers over the
// entry-compile logic in links.go (recompileLinks): this file adds no
// second implementation of "how to compile one entry's links" — the
// save-triggered path (entries.go's Create/Update) and CompileProject below
// both call the same recompileLinks function.
type LinksHandler struct {
	db    *sql.DB
	blobs BlobStore
}

func NewLinks(db *sql.DB, blobs BlobStore) *LinksHandler {
	return &LinksHandler{db: db, blobs: blobs}
}

// entryLinkView is one outgoing link as returned by GetForEntry — the
// entry_links row shape, JSON-cased for the API.
type entryLinkView struct {
	TargetPath    string `json:"targetPath"`
	TargetEntryID string `json:"targetEntryId,omitempty"`
	Label         string `json:"label"`
	CardFacet     string `json:"cardFacet,omitempty"`
	Resolved      bool   `json:"resolved"`
}

// GetForEntry godoc
// @Summary      List an entry's compiled outgoing links
// @Description  Reads entry_links as last compiled (on create/update, or via project compile) — does not re-parse the body.
// @Tags         links
// @Produce      json
// @Param        id   path      string  true  "Entry ID"
// @Success      200  {array}   entryLinkView
// @Failure      404  {object}  ErrorResponse
// @Router       /content/entries/{id}/links [get]
func (h *LinksHandler) GetForEntry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing entry id")
		return
	}

	var exists int
	if err := h.db.QueryRowContext(r.Context(), `SELECT 1 FROM entries WHERE id = ?`, id).Scan(&exists); err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	links, err := entryLinksFor(r.Context(), h.db, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, links)
}

// compileResponse is CompileProject's response body: how many entries were
// re-parsed and re-compiled, plus every dangling link found across the
// project — recorded as warnings, never as an error (docs/cli-spec.md §7
// "compile").
type compileResponse struct {
	ProjectID       string                `json:"projectId"`
	EntriesCompiled int                   `json:"entriesCompiled"`
	Warnings        []danglingLinkWarning `json:"warnings"`
}

// danglingLinkWarning names one unresolved outgoing link surfaced by a
// project compile.
type danglingLinkWarning struct {
	EntryID    string `json:"entryId"`
	TargetPath string `json:"targetPath"`
	Label      string `json:"label"`
}

// CompileProject godoc
// @Summary      Recompile entry_links for every entry in a project
// @Description  Re-parses each entry's body and reuses the same recompileLinks the save path calls. Dangling links are recorded and returned as warnings, never as an error — running this twice in a row is idempotent.
// @Tags         links
// @Produce      json
// @Param        id   path      string  true  "Project ID"
// @Success      200  {object}  compileResponse
// @Failure      503  {object}  ErrorResponse
// @Router       /content/projects/{id}/compile [post]
func (h *LinksHandler) CompileProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "missing project id")
		return
	}
	if h.blobs == nil {
		writeError(w, http.StatusServiceUnavailable, "blob storage not configured")
		return
	}

	entryIDs, err := entryIDsForProject(r.Context(), h.db, projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer tx.Rollback()

	for _, id := range entryIDs {
		data, err := h.blobs.Get(r.Context(), bodyKey(id))
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("fetch body for entry %s: %v", id, err))
			return
		}
		parsed, err := parse.Parse(data)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("parse entry %s: %v", id, err))
			return
		}
		// Same per-entry compile function the save-triggered path
		// (entries.go Create/Update) calls — not reimplemented here.
		if err := recompileLinks(r.Context(), tx, id, projectID, parsed); err != nil {
			writeError(w, http.StatusInternalServerError, "db error")
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	warnings, err := danglingLinksForProject(r.Context(), h.db, projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	writeJSON(w, http.StatusOK, compileResponse{
		ProjectID:       projectID,
		EntriesCompiled: len(entryIDs),
		Warnings:        warnings,
	})
}

// entryLinksFor returns entryID's entry_links rows as entryLinkView, in
// insertion (rowid) order.
func entryLinksFor(ctx context.Context, db *sql.DB, entryID string) ([]entryLinkView, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT target_path, COALESCE(target_entry_id, ''), label, COALESCE(card_facet, ''), resolved
		FROM entry_links
		WHERE entry_id = ?
		ORDER BY rowid
	`, entryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	links := []entryLinkView{}
	for rows.Next() {
		var l entryLinkView
		if err := rows.Scan(&l.TargetPath, &l.TargetEntryID, &l.Label, &l.CardFacet, &l.Resolved); err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	return links, rows.Err()
}

// entryIDsForProject returns the ids of every entry in projectID.
func entryIDsForProject(ctx context.Context, db *sql.DB, projectID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT id FROM entries WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// danglingLinksForProject returns every unresolved outgoing link belonging
// to an entry in projectID — the warnings a compile response surfaces.
func danglingLinksForProject(ctx context.Context, db *sql.DB, projectID string) ([]danglingLinkWarning, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT el.entry_id, el.target_path, el.label
		FROM entry_links el
		JOIN entries e ON e.id = el.entry_id
		WHERE e.project_id = ? AND el.resolved = 0
		ORDER BY el.entry_id, el.target_path
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	warnings := []danglingLinkWarning{}
	for rows.Next() {
		var wLink danglingLinkWarning
		if err := rows.Scan(&wLink.EntryID, &wLink.TargetPath, &wLink.Label); err != nil {
			return nil, err
		}
		warnings = append(warnings, wLink)
	}
	return warnings, rows.Err()
}
