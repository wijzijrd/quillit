package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/quillit/contentengine/parse"

	"github.com/quillit/content-svc/internal/authz"
)

// AssignHandler serves docs/web-refactor-spec.md §6.4/§7.2's assign endpoint
// — the web analogue of the CLI's `assign` command (docs/cli-spec.md §7):
// moving an entry to a new directory_path. Kept as its own handler type/file
// the same way render.go and links_handler.go are split out from
// entries.go/links.go: a distinct HTTP-facing concern, built on top of
// entries.go/links.go's existing helpers (recompileLinks, splitEntryPath/
// joinEntryPath, fetchResolvedEntry) rather than reimplementing them.
type AssignHandler struct {
	db        *sql.DB
	blobs     BlobStore
	jwtSecret []byte
	checker   authz.Checker
}

func NewAssign(db *sql.DB, blobs BlobStore, jwtSecret string, checker authz.Checker) *AssignHandler {
	return &AssignHandler{db: db, blobs: blobs, jwtSecret: []byte(jwtSecret), checker: checker}
}

type assignRequest struct {
	DirectoryPath string `json:"directory_path"`
}

// Assign godoc
// @Summary      Move an entry to a new directory_path
// @Description  No directories table exists (paths are just strings on entries — refactor-spec §4.4), so moving to a path that "doesn't exist yet" needs no explicit creation. Fails clearly (409) on a slug collision at the destination, with no partial state change. On success, every entry_links row that pointed at the entry's old path is rewritten to its new path — a deliberate web-only divergence from the CLI's dangling-link-report-only behavior; see refactor-spec §10.6.
// @Tags         entries
// @Accept       json
// @Produce      json
// @Param        id    path  string         true  "Entry ID"
// @Param        body  body  assignRequest  true  "Destination directory_path"
// @Success      200   {object}  Entry
// @Failure      400   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Failure      409   {object}  ErrorResponse
// @Router       /content/entries/{id}/assign [post]
func (h *AssignHandler) Assign(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing entry id")
		return
	}

	existing, err := scanEntryMeta(h.db.QueryRowContext(r.Context(), entryMetaSelect+" WHERE id = ?", id))
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if _, ok := requireProjectMember(w, r, h.jwtSecret, h.checker, existing.ProjectID); !ok {
		return
	}

	var req assignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}

	oldPath := joinEntryPath(existing.DirectoryPath, existing.Slug)
	newPath := joinEntryPath(req.DirectoryPath, existing.Slug)

	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer tx.Rollback()

	// Slug-collision check at the destination, up front and inside the same
	// transaction as the move — matching CLI spec §7 "assign": "Fails
	// clearly if... destination has same-name entry." Excludes this entry's
	// own row so re-assigning to the same directory (a no-op move) never
	// collides with itself.
	var collision int
	switch err := tx.QueryRowContext(r.Context(), `
		SELECT 1 FROM entries WHERE project_id = ? AND directory_path = ? AND slug = ? AND id != ?
	`, existing.ProjectID, req.DirectoryPath, existing.Slug, id).Scan(&collision); {
	case err == nil:
		writeError(w, http.StatusConflict, fmt.Sprintf("an entry already exists at %s", newPath))
		return
	case err != sql.ErrNoRows:
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	now := nowUnix()
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE entries SET directory_path = ?, updated_at = ? WHERE id = ?
	`, req.DirectoryPath, now, id); err != nil {
		// Defense in depth alongside the pre-check above: the (project_id,
		// directory_path, slug) UNIQUE constraint (internal/db/migrations/
		// 00001_baseline.sql) is the last word on collisions.
		if isUniqueConstraintErr(err) {
			writeError(w, http.StatusConflict, fmt.Sprintf("an entry already exists at %s", newPath))
			return
		}
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	// Inbound-link auto-rewrite — deliberate web-only divergence from the
	// CLI, called out in docs/web-refactor-spec.md §10.6: the CLI only
	// *reports* dangling links after a move (it has no cheap way to find
	// every inbound link across a project — no global index, only per-entry
	// links.conf files). The web app has entry_links as a live global index,
	// so it can and does rewrite every inbound link's target_path in place.
	// A targeted UPDATE (not delete+reinsert) so label and card_facet on the
	// rewritten rows are preserved untouched, and target_entry_id/resolved
	// stay correct as-is — the entry these rows point at hasn't changed,
	// only the path string used to address it. Scoped to this entry's own
	// project via the subquery: target_path strings aren't unique across
	// projects, so an unscoped UPDATE could rewrite an unrelated project's
	// coincidentally-identical path.
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE entry_links SET target_path = ?
		WHERE target_path = ? AND entry_id IN (SELECT id FROM entries WHERE project_id = ?)
	`, newPath, oldPath, existing.ProjectID); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	// Recompile the moved entry's own outgoing links, via the same
	// save-triggered recompileLinks entries.go's Create/Update call — so
	// this entry's entry_links rows can never silently drift from its body
	// content, matching every other write path's compile-on-save rule.
	//
	// In practice this is a no-op for a directory-only move: wikilink
	// targets are project-root-relative paths (refactor-spec §4.1), not
	// relative to the linking entry's own location, so the moved entry's
	// body text — and therefore its extracted outgoing link targets — is
	// unaffected by its own move. Verified by
	// TestAssign_RecompilesMovedEntrysOwnOutgoingLinks_NoOpSincePathsAreProjectRootRelative
	// rather than assumed. Skipped only if blob storage isn't configured or
	// the body can't be fetched/parsed — assign must not fail just because
	// this always-a-no-op step couldn't run.
	if h.blobs != nil {
		if data, err := h.blobs.Get(r.Context(), bodyKey(id)); err == nil {
			if parsed, perr := parse.Parse(data); perr == nil {
				if err := recompileLinks(r.Context(), tx, id, existing.ProjectID, parsed); err != nil {
					writeError(w, http.StatusInternalServerError, "db error")
					return
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	e, err := fetchResolvedEntry(r.Context(), h.db, h.blobs, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, e)
}
