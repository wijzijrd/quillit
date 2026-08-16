package handler

import (
	"context"
	nethtml "html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/quillit/contentengine/filter"
	"github.com/quillit/contentengine/render"

	"github.com/quillit/content-svc/internal/authz"
)

func newTestExportHandler(t *testing.T) (*EntriesHandler, *ExportHandler) {
	t.Helper()
	entries, blobs := newTestEntriesHandler(t)
	return entries, NewExport(entries.db, blobs, "", authz.AllowAll{})
}

// exportHandlerDenyingEveryone shares database and blobs with an
// already-populated ExportHandler/EntriesHandler pair, but answers "not a
// member" for every (user, project) pair — for the auth-rejection tests
// below (#44 retrofit).
func exportHandlerDenyingEveryone(entries *EntriesHandler) *ExportHandler {
	return NewExport(entries.db, entries.blobs, "", authz.Static{})
}

func exportEntry(t *testing.T, h *ExportHandler, id, query string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/content/entries/" + id + "/export"
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = withChiParam(req, "id", id)
	w := httptest.NewRecorder()
	h.Export(w, req)
	return w
}

func TestExport_SingleEntry_ValidPDF(t *testing.T) {
	entries, h := newTestExportHandler(t)
	e := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "tom", Body: "Just prose about Tom."})

	w := exportEntry(t, h, e.ID, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q, want application/pdf", ct)
	}
	if !strings.HasPrefix(w.Body.String(), "%PDF-") {
		body := w.Body.String()
		if len(body) > 20 {
			body = body[:20]
		}
		t.Errorf("body doesn't look like a PDF, first bytes: %q", body)
	}
	if w.Body.Len() < 500 {
		t.Errorf("suspiciously small PDF: %d bytes", w.Body.Len())
	}
}

func TestExport_UnknownEntry_404(t *testing.T) {
	_, h := newTestExportHandler(t)
	w := exportEntry(t, h, "does-not-exist", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", w.Code, w.Body.String())
	}
}

func TestExport_ViewAndCardMutuallyExclusive(t *testing.T) {
	entries, h := newTestExportHandler(t)
	e := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "tom", Body: "Prose."})

	w := exportEntry(t, h, e.ID, "view=player&card=motivation")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", w.Code, w.Body.String())
	}
}

func TestExport_UnknownFacet_422(t *testing.T) {
	entries, h := newTestExportHandler(t)
	e := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "tom", Body: "Prose only, no cards."})

	w := exportEntry(t, h, e.ID, "card=role")
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", w.Code, w.Body.String())
	}
}

func TestExport_With_UnknownBundledEntry_404(t *testing.T) {
	entries, h := newTestExportHandler(t)
	e := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "tom", Body: "Prose."})

	w := exportEntry(t, h, e.ID, "with=does-not-exist")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", w.Code, w.Body.String())
	}
}

func TestExport_Bundle_MultipleEntries_ValidPDF(t *testing.T) {
	entries, h := newTestExportHandler(t)
	mary := createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "mary", DirectoryPath: "characters/npcs", Body: "Mary runs the bakery.",
	})
	tom := createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "tom", DirectoryPath: "characters/npcs",
		Body: "Tom talks about [[characters/npcs/mary|Mary]] often.",
	})

	w := exportEntry(t, h, tom.ID, "with="+mary.ID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if !strings.HasPrefix(w.Body.String(), "%PDF-") {
		t.Errorf("body doesn't look like a PDF")
	}

	// A two-entry bundle should produce visibly more content (two pages)
	// than a single-entry export of just the main entry.
	single := exportEntry(t, h, tom.ID, "")
	if w.Body.Len() <= single.Body.Len() {
		t.Errorf("bundled PDF (%d bytes) not larger than single-entry PDF (%d bytes)", w.Body.Len(), single.Body.Len())
	}
}

func TestExport_WithMultipleIDs_CommaSeparated(t *testing.T) {
	entries, h := newTestExportHandler(t)
	mary := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary."})
	ghost := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "ghost", Body: "A ghost."})
	tom := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "tom", Body: "Tom."})

	w := exportEntry(t, h, tom.ID, "with="+mary.ID+","+ghost.ID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

// --- Content-correctness checks, done at the pre-PDF fragment layer.
//
// Real PDF bytes embed the font as glyph indices (Identity-H composite
// font), not literal characters, so grepping the HTTP response body for
// plain text never works regardless of secrets/links — verified against
// this codebase's own export.Bundle output, and the reason
// pkg/contentengine/export/export_test.go asserts on the HTML fragment
// (via a captureSink) rather than on PDF bytes. These tests do the same
// thing here: drive ExportHandler's own entry-resolution step (the part
// this handler owns — meta lookup, view parsing, vocabulary, &with=
// bundling) and inspect the resulting filter.FilteredEntry the same way
// export.Bundle would render it (render.Render, resolver=nil, a
// plain-label-only LinkRenderer — the identical call export.go's
// renderFragment makes).

func mustResolveExportEntries(t *testing.T, h *ExportHandler, mainID, query string) []*filter.FilteredEntry {
	t.Helper()
	url := "/content/entries/" + mainID + "/export"
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = withChiParam(req, "id", mainID)

	_, entries, ferr := h.resolveExportEntries(req)
	if ferr != nil {
		t.Fatalf("resolveExportEntries: status=%d msg=%s", ferr.status, ferr.msg)
	}
	return entries
}

// renderAsPDFWould mirrors export.Bundle's internal renderFragment exactly:
// resolver=nil (never expand cards/links) and a LinkRenderer that returns
// only the escaped label (no markup) — CLI spec §4's "plain text label
// only" rule.
func renderAsPDFWould(t *testing.T, entry *filter.FilteredEntry) string {
	t.Helper()
	fragment, err := render.Render(entry, filter.View{}, nil, func(info render.LinkInfo) string {
		return nethtml.EscapeString(info.Label)
	})
	if err != nil {
		t.Fatalf("render.Render: %v", err)
	}
	return fragment
}

func TestExportEntries_PlayerView_StripsSecretsFromMainEntry(t *testing.T) {
	entries, h := newTestExportHandler(t)
	e := createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "tom",
		Body: "Public intro.\n\n:::secret\nTom is secretly a spy for the Crimson Hand.\n:::\n",
	})

	filtered := mustResolveExportEntries(t, h, e.ID, "view=player")
	if len(filtered) != 1 {
		t.Fatalf("got %d entries, want 1", len(filtered))
	}
	fragment := renderAsPDFWould(t, filtered[0])
	if strings.Contains(fragment, "Crimson Hand") {
		t.Errorf("player-view export leaked secret text: %q", fragment)
	}
	if !strings.Contains(fragment, "Public intro") {
		t.Errorf("player-view export dropped non-secret content: %q", fragment)
	}
}

func TestExportEntries_Bundle_PlayerView_StripsSecretsFromEveryEntry(t *testing.T) {
	entries, h := newTestExportHandler(t)
	main := createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "tom", DirectoryPath: "characters/npcs",
		Body: "Tom talks about [[characters/npcs/mary|Mary]].",
	})
	bundled := createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "mary", DirectoryPath: "characters/npcs",
		Body: "Public bakery owner.\n\n:::secret\nMary is secretly the Crimson Hand's spymaster.\n:::\n",
	})

	filtered := mustResolveExportEntries(t, h, main.ID, "view=player&with="+bundled.ID)
	if len(filtered) != 2 {
		t.Fatalf("got %d entries, want 2 (main + bundled)", len(filtered))
	}

	// The bundled (second) entry specifically — not just the main one —
	// must have its secret stripped. This is the explicit regression the
	// issue calls out: player view must strip secrets from every bundled
	// entry, not just the main one.
	bundledFragment := renderAsPDFWould(t, filtered[1])
	if strings.Contains(bundledFragment, "Crimson Hand") {
		t.Errorf("bundled entry leaked secret text in player-view export: %q", bundledFragment)
	}
	if !strings.Contains(bundledFragment, "Public bakery owner") {
		t.Errorf("bundled entry dropped non-secret content: %q", bundledFragment)
	}
}

func TestExportEntries_DMView_LinksArePlainTextOnly(t *testing.T) {
	entries, h := newTestExportHandler(t)
	createEntry(t, entries, "proj-1", createEntryRequest{Slug: "mary", DirectoryPath: "characters/npcs", Body: "Mary."})
	tom := createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "tom", DirectoryPath: "characters/npcs",
		Body: "Tom talks about [[characters/npcs/mary|Mary]] often.\n",
	})

	filtered := mustResolveExportEntries(t, h, tom.ID, "") // default DM view
	fragment := renderAsPDFWould(t, filtered[0])

	if !strings.Contains(fragment, "Mary") {
		t.Errorf("expected the link label \"Mary\" as plain text: %q", fragment)
	}
	if strings.Contains(fragment, "<a ") || strings.Contains(fragment, "data-quillit-link") {
		t.Errorf("PDF export must never contain interactive link markup, got: %q", fragment)
	}
	if strings.Contains(fragment, "wikilink") {
		t.Errorf("PDF export must never contain the web renderer's wikilink class, got: %q", fragment)
	}
}

func TestExportEntries_CardView_AppliedToBundledEntriesToo(t *testing.T) {
	entries, h := newTestExportHandler(t)
	main := createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "tom",
		Body: ":::card motivation\nWants his farm back.\n:::\n",
	})
	bundled := createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "mary",
		Body: ":::card motivation\nWants revenge on the duke.\n:::\n",
	})

	filtered := mustResolveExportEntries(t, h, main.ID, "card=motivation&with="+bundled.ID)
	if len(filtered) != 2 {
		t.Fatalf("got %d entries, want 2", len(filtered))
	}
	mainFragment := renderAsPDFWould(t, filtered[0])
	bundledFragment := renderAsPDFWould(t, filtered[1])
	if !strings.Contains(mainFragment, "farm back") {
		t.Errorf("main entry missing its motivation card: %q", mainFragment)
	}
	if !strings.Contains(bundledFragment, "revenge on the duke") {
		t.Errorf("bundled entry missing its motivation card — card view wasn't applied uniformly: %q", bundledFragment)
	}
}

// ── Auth (#44) ───────────────────────────────────────────────────────────

func TestExport_RejectsUnauthenticatedRequest(t *testing.T) {
	entries, h := newTestExportHandler(t)
	e := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "solo", Body: "Just prose."})

	req := httptest.NewRequest(http.MethodGet, "/content/entries/"+e.ID+"/export", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", e.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx)) // no caller injected
	w := httptest.NewRecorder()
	h.Export(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestExport_RejectsNonProjectMember(t *testing.T) {
	entries, _ := newTestExportHandler(t)
	e := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "solo", Body: "Just prose."})

	denied := exportHandlerDenyingEveryone(entries)
	w := exportEntry(t, denied, e.ID, "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}
