package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/quillit/content-svc/internal/authz"
)

func newTestLinksHandler(t *testing.T) (*LinksHandler, *EntriesHandler) {
	t.Helper()
	entries, blobs := newTestEntriesHandler(t)
	return NewLinks(entries.db, blobs, "", authz.AllowAll{}), entries
}

// linksHandlerDenyingEveryone shares database and blobs with an
// already-populated LinksHandler/EntriesHandler pair, but answers "not a
// member" for every (user, project) pair — for the auth-rejection tests
// below.
func linksHandlerDenyingEveryone(entries *EntriesHandler) *LinksHandler {
	return NewLinks(entries.db, entries.blobs, "", authz.Static{})
}

func getEntryLinks(t *testing.T, h *LinksHandler, entryID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/content/entries/"+entryID+"/links", nil)
	req = withChiParam(req, "id", entryID)
	w := httptest.NewRecorder()
	h.GetForEntry(w, req)
	return w
}

func compileProject(t *testing.T, h *LinksHandler, projectID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/content/projects/"+projectID+"/compile", nil)
	req = withChiParam(req, "id", projectID)
	w := httptest.NewRecorder()
	h.CompileProject(w, req)
	return w
}

func TestGetForEntry_ReturnsResolvedAndDanglingLinks(t *testing.T) {
	h, entries := newTestLinksHandler(t)
	createEntry(t, entries, "proj-1", createEntryRequest{Slug: "mary", DirectoryPath: "characters/npcs", Body: "Mary."})
	tom := createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "tom", DirectoryPath: "characters/npcs",
		Body: "Tom talks about [[characters/npcs/mary|Mary]] and [[characters/npcs/ghost|Ghost]] often.",
	})

	w := getEntryLinks(t, h, tom.ID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var links []entryLinkView
	if err := json.Unmarshal(w.Body.Bytes(), &links); err != nil {
		t.Fatalf("decode links: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("got %d links, want 2: %+v", len(links), links)
	}

	byTarget := map[string]entryLinkView{}
	for _, l := range links {
		byTarget[l.TargetPath] = l
	}

	mary, ok := byTarget["characters/npcs/mary"]
	if !ok {
		t.Fatalf("expected a link to characters/npcs/mary, got %+v", links)
	}
	if !mary.Resolved {
		t.Errorf("mary link Resolved = false, want true")
	}
	if mary.Label != "Mary" {
		t.Errorf("mary link Label = %q, want Mary", mary.Label)
	}

	ghost, ok := byTarget["characters/npcs/ghost"]
	if !ok {
		t.Fatalf("expected a dangling link to characters/npcs/ghost, got %+v", links)
	}
	if ghost.Resolved {
		t.Errorf("ghost link Resolved = true, want false (dangling)")
	}
}

func TestGetForEntry_UnknownEntryIs404(t *testing.T) {
	h, _ := newTestLinksHandler(t)
	w := getEntryLinks(t, h, "does-not-exist")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestGetForEntry_NoLinksReturnsEmptyArray(t *testing.T) {
	h, entries := newTestLinksHandler(t)
	e := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "solo", Body: "No links here."})

	w := getEntryLinks(t, h, e.ID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var links []entryLinkView
	if err := json.Unmarshal(w.Body.Bytes(), &links); err != nil {
		t.Fatalf("decode links: %v", err)
	}
	if links == nil {
		t.Error("expected an empty array, not null, for an entry with no outgoing links")
	}
	if len(links) != 0 {
		t.Errorf("got %d links, want 0", len(links))
	}
}

func TestCompileProject_RecompilesEveryEntryAndIsIdempotent(t *testing.T) {
	h, entries := newTestLinksHandler(t)
	mary := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "mary", DirectoryPath: "characters/npcs", Body: "Mary."})
	tom := createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "tom", DirectoryPath: "characters/npcs",
		Body: "Tom talks about [[characters/npcs/mary|Mary]] often.",
	})
	// A second, unrelated project must not be touched by proj-1's compile.
	createEntry(t, entries, "proj-2", createEntryRequest{Slug: "other", Body: "Nothing to see here."})

	w1 := compileProject(t, h, "proj-1")
	if w1.Code != http.StatusOK {
		t.Fatalf("first compile: status = %d, body = %s", w1.Code, w1.Body.String())
	}
	var resp1 compileResponse
	if err := json.Unmarshal(w1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("decode compile response: %v", err)
	}
	if resp1.EntriesCompiled != 2 {
		t.Errorf("EntriesCompiled = %d, want 2 (mary + tom)", resp1.EntriesCompiled)
	}

	// Confirm the resolved link landed as expected.
	var targetID string
	var resolved bool
	if err := h.db.QueryRow(`SELECT target_entry_id, resolved FROM entry_links WHERE entry_id = ?`, tom.ID).Scan(&targetID, &resolved); err != nil {
		t.Fatalf("expected an entry_links row for tom: %v", err)
	}
	if targetID != mary.ID || !resolved {
		t.Errorf("entry_links row = (target=%s, resolved=%v), want (target=%s, resolved=true)", targetID, resolved, mary.ID)
	}

	// Compiling again must produce the exact same result (idempotent).
	w2 := compileProject(t, h, "proj-1")
	if w2.Code != http.StatusOK {
		t.Fatalf("second compile: status = %d, body = %s", w2.Code, w2.Body.String())
	}
	var resp2 compileResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("decode compile response: %v", err)
	}
	if resp2.EntriesCompiled != resp1.EntriesCompiled {
		t.Errorf("second compile EntriesCompiled = %d, want %d (same as first)", resp2.EntriesCompiled, resp1.EntriesCompiled)
	}
	if len(resp2.Warnings) != len(resp1.Warnings) {
		t.Errorf("second compile Warnings = %+v, want same as first %+v", resp2.Warnings, resp1.Warnings)
	}

	var countAfter int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM entry_links WHERE entry_id = ?`, tom.ID).Scan(&countAfter); err != nil {
		t.Fatalf("count entry_links: %v", err)
	}
	if countAfter != 1 {
		t.Errorf("entry_links count for tom after second compile = %d, want 1 (no duplicate rows)", countAfter)
	}
}

func TestCompileProject_DanglingLinkIsWarningNotError(t *testing.T) {
	h, entries := newTestLinksHandler(t)
	createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "tom", DirectoryPath: "characters/npcs",
		Body: "Tom mentions [[characters/npcs/ghost|a ghost]] who doesn't exist.",
	})

	w := compileProject(t, h, "proj-1")
	if w.Code < 200 || w.Code >= 300 {
		t.Fatalf("status = %d, want 2xx even with a dangling link, body = %s", w.Code, w.Body.String())
	}

	var resp compileResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode compile response: %v", err)
	}
	if len(resp.Warnings) != 1 {
		t.Fatalf("Warnings = %+v, want exactly 1 dangling link warning", resp.Warnings)
	}
	if resp.Warnings[0].TargetPath != "characters/npcs/ghost" {
		t.Errorf("warning TargetPath = %q, want characters/npcs/ghost", resp.Warnings[0].TargetPath)
	}
}

func TestCompileProject_EmptyProjectSucceeds(t *testing.T) {
	h, _ := newTestLinksHandler(t)
	w := compileProject(t, h, "no-such-project")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp compileResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode compile response: %v", err)
	}
	if resp.EntriesCompiled != 0 {
		t.Errorf("EntriesCompiled = %d, want 0", resp.EntriesCompiled)
	}
	if len(resp.Warnings) != 0 {
		t.Errorf("Warnings = %+v, want none", resp.Warnings)
	}
}

// ── Auth (#44) ───────────────────────────────────────────────────────────

func TestGetForEntry_RejectsUnauthenticatedRequest(t *testing.T) {
	h, entries := newTestLinksHandler(t)
	e := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "solo", Body: "No links."})

	req := httptest.NewRequest(http.MethodGet, "/content/entries/"+e.ID+"/links", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", e.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx)) // no caller injected
	w := httptest.NewRecorder()
	h.GetForEntry(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestGetForEntry_RejectsNonProjectMember(t *testing.T) {
	_, entries := newTestLinksHandler(t)
	e := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "solo", Body: "No links."})

	denied := linksHandlerDenyingEveryone(entries)
	w := getEntryLinks(t, denied, e.ID)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestCompileProject_RejectsNonProjectMember(t *testing.T) {
	_, entries := newTestLinksHandler(t)
	createEntry(t, entries, "proj-1", createEntryRequest{Slug: "tom", Body: "Tom."})

	denied := linksHandlerDenyingEveryone(entries)
	w := compileProject(t, denied, "proj-1")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}
