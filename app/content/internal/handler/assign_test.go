package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/quillit/content-svc/internal/authz"
)

func newTestAssignHandler(t *testing.T) (*AssignHandler, *EntriesHandler) {
	t.Helper()
	entries, blobs := newTestEntriesHandler(t)
	return NewAssign(entries.db, blobs, "", authz.AllowAll{}), entries
}

// assignHandlerDenyingEveryone shares database and blobs with an
// already-populated AssignHandler/EntriesHandler pair, but answers "not a
// member" for every (user, project) pair — for the auth-rejection tests
// below. Mirrors links_handler_test.go's linksHandlerDenyingEveryone.
func assignHandlerDenyingEveryone(entries *EntriesHandler) *AssignHandler {
	return NewAssign(entries.db, entries.blobs, "", authz.Static{})
}

func assignEntry(t *testing.T, h *AssignHandler, entryID, directoryPath string) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"directory_path": directoryPath})
	req := httptest.NewRequest(http.MethodPost, "/content/entries/"+entryID+"/assign", strings.NewReader(string(raw)))
	req = withChiParam(req, "id", entryID)
	w := httptest.NewRecorder()
	h.Assign(w, req)
	return w
}

func TestAssign_MovesEntry_NoCollision(t *testing.T) {
	h, entries := newTestAssignHandler(t)
	mary := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "mary", DirectoryPath: "characters/npcs", Body: "Mary."})

	w := assignEntry(t, h, mary.ID, "characters/villains")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var e Entry
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if e.DirectoryPath != "characters/villains" {
		t.Errorf("DirectoryPath = %q, want %q", e.DirectoryPath, "characters/villains")
	}

	var dbDir string
	if err := entries.db.QueryRow(`SELECT directory_path FROM entries WHERE id = ?`, mary.ID).Scan(&dbDir); err != nil {
		t.Fatalf("query directory_path: %v", err)
	}
	if dbDir != "characters/villains" {
		t.Errorf("db directory_path = %q, want %q", dbDir, "characters/villains")
	}
}

func TestAssign_SlugCollision_FailsWithNoPartialStateChange(t *testing.T) {
	h, entries := newTestAssignHandler(t)
	mary := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "mary", DirectoryPath: "characters/npcs", Body: "Mary."})
	createEntry(t, entries, "proj-1", createEntryRequest{Slug: "mary", DirectoryPath: "characters/villains", Body: "Evil Mary."})

	// Snapshot every relevant row before attempting the doomed move.
	var beforeDir string
	var beforeUpdatedAt int64
	if err := entries.db.QueryRow(`SELECT directory_path, updated_at FROM entries WHERE id = ?`, mary.ID).Scan(&beforeDir, &beforeUpdatedAt); err != nil {
		t.Fatalf("snapshot entries row: %v", err)
	}
	var beforeCount int
	if err := entries.db.QueryRow(`SELECT COUNT(*) FROM entries WHERE project_id = ?`, "proj-1").Scan(&beforeCount); err != nil {
		t.Fatalf("snapshot entries count: %v", err)
	}

	w := assignEntry(t, h, mary.ID, "characters/villains")
	if w.Code < 400 || w.Code >= 500 {
		t.Fatalf("status = %d, want a 4xx collision error, body = %s", w.Code, w.Body.String())
	}

	var afterDir string
	var afterUpdatedAt int64
	if err := entries.db.QueryRow(`SELECT directory_path, updated_at FROM entries WHERE id = ?`, mary.ID).Scan(&afterDir, &afterUpdatedAt); err != nil {
		t.Fatalf("re-query entries row: %v", err)
	}
	if afterDir != beforeDir {
		t.Errorf("directory_path changed to %q despite collision failure, want unchanged %q", afterDir, beforeDir)
	}
	if afterUpdatedAt != beforeUpdatedAt {
		t.Errorf("updated_at changed despite collision failure — partial state change")
	}
	var afterCount int
	if err := entries.db.QueryRow(`SELECT COUNT(*) FROM entries WHERE project_id = ?`, "proj-1").Scan(&afterCount); err != nil {
		t.Fatalf("re-query entries count: %v", err)
	}
	if afterCount != beforeCount {
		t.Errorf("entries count changed from %d to %d despite collision failure", beforeCount, afterCount)
	}
}

func TestAssign_RewritesInboundLinks(t *testing.T) {
	h, entries := newTestAssignHandler(t)
	mary := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "mary", DirectoryPath: "characters/npcs", Body: "Mary the merchant."})
	tom := createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "tom", DirectoryPath: "characters/npcs",
		Body: "Tom talks about [[characters/npcs/mary|Mary]] often.",
	})
	sara := createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "sara", DirectoryPath: "characters/npcs",
		Body: "Sara also mentions [[characters/npcs/mary|the merchant]].",
	})

	w := assignEntry(t, h, mary.ID, "characters/villains")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	const newPath = "characters/villains/mary"

	var tomTarget, tomLabel string
	var tomTargetID string
	var tomResolved bool
	if err := entries.db.QueryRow(`SELECT target_path, COALESCE(target_entry_id,''), label, resolved FROM entry_links WHERE entry_id = ?`, tom.ID).
		Scan(&tomTarget, &tomTargetID, &tomLabel, &tomResolved); err != nil {
		t.Fatalf("query tom's entry_links row: %v", err)
	}
	if tomTarget != newPath {
		t.Errorf("tom's link target_path = %q, want %q", tomTarget, newPath)
	}
	if tomLabel != "Mary" {
		t.Errorf("tom's link label = %q, want %q (preserved)", tomLabel, "Mary")
	}
	if tomTargetID != mary.ID || !tomResolved {
		t.Errorf("tom's link target_entry_id/resolved = (%s,%v), want (%s,true)", tomTargetID, tomResolved, mary.ID)
	}

	var saraTarget, saraLabel string
	if err := entries.db.QueryRow(`SELECT target_path, label FROM entry_links WHERE entry_id = ?`, sara.ID).
		Scan(&saraTarget, &saraLabel); err != nil {
		t.Fatalf("query sara's entry_links row: %v", err)
	}
	if saraTarget != newPath {
		t.Errorf("sara's link target_path = %q, want %q", saraTarget, newPath)
	}
	if saraLabel != "the merchant" {
		t.Errorf("sara's link label = %q, want %q (preserved)", saraLabel, "the merchant")
	}
}

func TestAssign_RewritesInboundLinkInsideCardBlock_PreservingCardFacet(t *testing.T) {
	h, entries := newTestAssignHandler(t)
	mary := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "mary", DirectoryPath: "characters/npcs", Body: "Mary the merchant."})
	tom := createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "tom", DirectoryPath: "characters/npcs",
		Body: ":::card motivation\nTom wants to find [[characters/npcs/mary|Mary]] before sundown.\n:::\n",
	})

	w := assignEntry(t, h, mary.ID, "characters/villains")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	const newPath = "characters/villains/mary"
	var target, label, cardFacet string
	if err := entries.db.QueryRow(`SELECT target_path, label, COALESCE(card_facet,'') FROM entry_links WHERE entry_id = ?`, tom.ID).
		Scan(&target, &label, &cardFacet); err != nil {
		t.Fatalf("query tom's entry_links row: %v", err)
	}
	if target != newPath {
		t.Errorf("target_path = %q, want %q", target, newPath)
	}
	if label != "Mary" {
		t.Errorf("label = %q, want %q (preserved)", label, "Mary")
	}
	if cardFacet != "motivation" {
		t.Errorf("card_facet = %q, want %q (preserved through rewrite)", cardFacet, "motivation")
	}
}

func TestAssign_DoesNotRewriteLinksInOtherProjects(t *testing.T) {
	h, entries := newTestAssignHandler(t)
	mary := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "mary", DirectoryPath: "characters/npcs", Body: "Mary."})
	// A different project happens to have an entry with a link to the exact
	// same path string — this must never be touched by proj-1's move.
	other := createEntry(t, entries, "proj-2", createEntryRequest{
		Slug: "unrelated", Body: "See also [[characters/npcs/mary|Mary]] (coincidental same path, different project).",
	})

	w := assignEntry(t, h, mary.ID, "characters/villains")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var target string
	if err := entries.db.QueryRow(`SELECT target_path FROM entry_links WHERE entry_id = ?`, other.ID).Scan(&target); err != nil {
		t.Fatalf("query other project's entry_links row: %v", err)
	}
	if target != "characters/npcs/mary" {
		t.Errorf("cross-project link target_path = %q, want unchanged %q", target, "characters/npcs/mary")
	}
}

func TestAssign_RecompilesMovedEntrysOwnOutgoingLinks_NoOpSincePathsAreProjectRootRelative(t *testing.T) {
	h, entries := newTestAssignHandler(t)
	createEntry(t, entries, "proj-1", createEntryRequest{Slug: "tavern", DirectoryPath: "locations", Body: "The tavern."})
	tom := createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "tom", DirectoryPath: "characters/npcs",
		Body: "Tom works at [[locations/tavern|the tavern]].",
	})

	var before struct {
		target, label, targetID string
		resolved                bool
	}
	if err := entries.db.QueryRow(`SELECT target_path, label, COALESCE(target_entry_id,''), resolved FROM entry_links WHERE entry_id = ?`, tom.ID).
		Scan(&before.target, &before.label, &before.targetID, &before.resolved); err != nil {
		t.Fatalf("query tom's entry_links row before move: %v", err)
	}

	w := assignEntry(t, h, tom.ID, "characters/villains")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var after struct {
		target, label, targetID string
		resolved                bool
	}
	if err := entries.db.QueryRow(`SELECT target_path, label, COALESCE(target_entry_id,''), resolved FROM entry_links WHERE entry_id = ?`, tom.ID).
		Scan(&after.target, &after.label, &after.targetID, &after.resolved); err != nil {
		t.Fatalf("query tom's entry_links row after move: %v", err)
	}

	if after.target != before.target || after.label != before.label || after.targetID != before.targetID || after.resolved != before.resolved {
		t.Errorf("tom's own outgoing link changed across a directory-only move: before=%+v after=%+v (wikilinks are project-root-relative, so this should be a no-op)", before, after)
	}
}

func TestAssign_UnknownEntryIs404(t *testing.T) {
	h, _ := newTestAssignHandler(t)
	w := assignEntry(t, h, "does-not-exist", "somewhere")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestAssign_SameDirectoryIsANoOp(t *testing.T) {
	h, entries := newTestAssignHandler(t)
	mary := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "mary", DirectoryPath: "characters/npcs", Body: "Mary."})

	w := assignEntry(t, h, mary.ID, "characters/npcs")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	var dbDir string
	if err := entries.db.QueryRow(`SELECT directory_path FROM entries WHERE id = ?`, mary.ID).Scan(&dbDir); err != nil {
		t.Fatalf("query directory_path: %v", err)
	}
	if dbDir != "characters/npcs" {
		t.Errorf("directory_path = %q, want unchanged %q", dbDir, "characters/npcs")
	}
}

// ── Auth (#44) ───────────────────────────────────────────────────────────

func TestAssign_RejectsUnauthenticatedRequest(t *testing.T) {
	h, entries := newTestAssignHandler(t)
	e := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary."})

	raw, _ := json.Marshal(map[string]string{"directory_path": "somewhere"})
	req := httptest.NewRequest(http.MethodPost, "/content/entries/"+e.ID+"/assign", strings.NewReader(string(raw)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", e.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx)) // no caller injected
	w := httptest.NewRecorder()
	h.Assign(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestAssign_RejectsNonProjectMember(t *testing.T) {
	_, entries := newTestAssignHandler(t)
	e := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary."})

	denied := assignHandlerDenyingEveryone(entries)
	w := assignEntry(t, denied, e.ID, "somewhere")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}
