package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/quillit/contentengine/export"
	"github.com/quillit/contentengine/filter"

	"github.com/quillit/content-svc/internal/authz"
)

// ExportHandler serves docs/cli-spec.md §7 "export" over HTTP: one entry
// (optionally bundled with linked entries, &with=<comma-separated-entry-
// ids>) rendered to a single PDF. It wraps the exact same parse -> filter
// step RenderHandler uses (loadAndFilterEntry, render.go) — the only
// difference from /render is the last step: export.Bundle instead of
// render.Render + an HTML Content-Type. Filtering/rendering logic itself
// is never duplicated here; export.Bundle (pkg/contentengine/export) is
// also what already enforces CLI spec §4's PDF rule — links never expand,
// never show interactive affordances, plain text label only — since it
// always calls render.Render with resolver=nil and a label-only
// LinkRenderer, regardless of which view produced the entry.
type ExportHandler struct {
	db        *sql.DB
	blobs     BlobStore
	jwtSecret []byte
	checker   authz.Checker
}

func NewExport(db *sql.DB, blobs BlobStore, jwtSecret string, checker authz.Checker) *ExportHandler {
	return &ExportHandler{db: db, blobs: blobs, jwtSecret: []byte(jwtSecret), checker: checker}
}

// Export godoc
// @Summary      Export an entry, optionally bundled with linked entries, as a PDF
// @Description  Same view/card filtering as /render (mutually exclusive); &with=<comma-separated-entry-ids> bundles additional entries into one combined PDF, main entry first, the same view/card applied to every included entry — player view strips secrets from every bundled entry, not just the main one.
// @Tags         export
// @Produce      application/pdf
// @Param        id     path   string  true   "Entry ID"
// @Param        view   query  string  false  "dm (default) or player"
// @Param        card   query  string  false  "facet name"
// @Param        with   query  string  false  "comma-separated entry IDs to bundle after the main entry"
// @Success      200    {file}    binary
// @Failure      400    {object}  ErrorResponse
// @Failure      404    {object}  ErrorResponse
// @Failure      422    {object}  ErrorResponse
// @Router       /content/entries/{id}/export [get]
func (h *ExportHandler) Export(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	projectID, err := projectIDForEntry(r.Context(), h.db, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if _, ok := requireProjectMember(w, r, h.jwtSecret, h.checker, projectID); !ok {
		return
	}

	main, entries, ferr := h.resolveExportEntries(r)
	if ferr != nil {
		ferr.write(w)
		return
	}

	pdf, err := export.Bundle(entries)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "export error")
		return
	}

	filename := main.Slug
	if filename == "" {
		filename = main.ID
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.pdf"`, filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdf)
}

// resolveExportEntries does everything Export needs up to (but not
// including) the PDF-encoding step: look up the main entry, parse its
// view/card query params (parseView, render.go — identical to /render's
// mutual-exclusion rule), compute the effective facet vocabulary once from
// the main entry's project, then load+filter the main entry followed by
// each &with= entry, in order — the exact document order export.Bundle
// puts them in (main entry first, then each bundled section). Every listed
// entry is filtered against the same view and the same vocabulary, so
// player-view secret-stripping and card-view facet selection apply
// uniformly across the whole bundle, not just the main entry.
//
// Split out from Export so tests can assert on the resulting
// filter.FilteredEntry values directly — the HTTP handler's real PDF
// bytes aren't text-searchable (the embedded font stores glyph indices,
// not literal characters; see export_test.go's package doc comment).
// It also returns the main entry's EntryMeta, so Export doesn't need a
// second lookup just to name the downloaded file.
func (h *ExportHandler) resolveExportEntries(r *http.Request) (EntryMeta, []*filter.FilteredEntry, *entryFilterError) {
	id := chi.URLParam(r, "id")

	m, err := scanEntryMeta(h.db.QueryRowContext(r.Context(), entryMetaSelect+" WHERE id = ?", id))
	if err != nil {
		return EntryMeta{}, nil, &entryFilterError{status: http.StatusNotFound, msg: "not found"}
	}
	if h.blobs == nil {
		return m, nil, &entryFilterError{status: http.StatusServiceUnavailable, msg: "blob storage not configured"}
	}

	view, ok := parseView(r)
	if !ok {
		return m, nil, &entryFilterError{status: http.StatusBadRequest, msg: "view and card are mutually exclusive"}
	}

	_, vocab, err := effectiveFacetVocabulary(r.Context(), h.db, m.ProjectID)
	if err != nil {
		return m, nil, &entryFilterError{status: http.StatusInternalServerError, msg: "db error"}
	}

	mainFiltered, ferr := loadAndFilterEntry(r.Context(), h.blobs, m, view, vocab)
	if ferr != nil {
		return m, nil, ferr
	}
	entries := []*filter.FilteredEntry{mainFiltered}

	withParam := strings.TrimSpace(r.URL.Query().Get("with"))
	if withParam == "" {
		return m, entries, nil
	}

	for _, withID := range splitCommaList(withParam) {
		wm, err := scanEntryMeta(h.db.QueryRowContext(r.Context(), entryMetaSelect+" WHERE id = ?", withID))
		if err != nil {
			return m, nil, &entryFilterError{status: http.StatusNotFound, msg: fmt.Sprintf("bundled entry %s not found", withID)}
		}
		filtered, ferr := loadAndFilterEntry(r.Context(), h.blobs, wm, view, vocab)
		if ferr != nil {
			return m, nil, &entryFilterError{status: ferr.status, unknownFacet: ferr.unknownFacet, msg: fmt.Sprintf("bundled entry %s: %s", withID, ferr.msg)}
		}
		entries = append(entries, filtered)
	}
	return m, entries, nil
}

// splitCommaList splits a comma-separated query param into trimmed,
// non-empty tokens, in order — the &with=<comma-separated-entry-ids> list
// format.
func splitCommaList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
