package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	_ "modernc.org/sqlite"

	"github.com/quillit/svc/internal/handler"
	"github.com/quillit/svc/internal/middleware"
)

// setupEntriesDB builds an in-memory DB with the subset of schema the entry
// read/write authorization predicate touches: entries plus the two relationship
// tables (entry_shares, project_members).
func setupEntriesDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	schema := `
	CREATE TABLE entries (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL DEFAULT '',
		category TEXT NOT NULL DEFAULT 'Lore',
		body TEXT NOT NULL DEFAULT '',
		body_key TEXT,
		visibility TEXT NOT NULL DEFAULT 'private',
		campaign_ids TEXT NOT NULL DEFAULT '[]',
		linked_entries TEXT NOT NULL DEFAULT '[]',
		tags TEXT NOT NULL DEFAULT '[]',
		quick_view_data TEXT NOT NULL DEFAULT '{}',
		created_at INTEGER NOT NULL DEFAULT 0,
		updated_at INTEGER NOT NULL DEFAULT 0,
		owner_user_id TEXT
	);
	CREATE TABLE entry_shares (
		id TEXT PRIMARY KEY,
		entry_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		shared_by TEXT NOT NULL,
		shared_at INTEGER NOT NULL,
		UNIQUE(entry_id, user_id)
	);
	CREATE TABLE project_members (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		role TEXT NOT NULL,
		joined_at INTEGER NOT NULL,
		username TEXT NOT NULL DEFAULT '',
		UNIQUE(project_id, user_id)
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	return db
}

// seedEntry inserts one entry with the given owner/visibility/campaign_ids.
func seedEntry(t *testing.T, db *sql.DB, id, owner, visibility, campaignIDs string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO entries (id, title, body, visibility, campaign_ids, owner_user_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, 1, 1)`,
		id, "Title "+id, "body "+id, visibility, campaignIDs, owner,
	); err != nil {
		t.Fatal(err)
	}
}

func shareEntry(t *testing.T, db *sql.DB, entryID, userID string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO entry_shares (id, entry_id, user_id, shared_by, shared_at) VALUES (?, ?, ?, ?, 1)`,
		"share-"+entryID+"-"+userID, entryID, userID, "sharer",
	); err != nil {
		t.Fatal(err)
	}
}

func addProjectMember(t *testing.T, db *sql.DB, projectID, userID string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO project_members (id, project_id, user_id, role, joined_at) VALUES (?, ?, ?, 'player', 1)`,
		"pm-"+projectID+"-"+userID, projectID, userID,
	); err != nil {
		t.Fatal(err)
	}
}

func entriesRouter(h *handler.EntriesHandler) chi.Router {
	r := chi.NewRouter()
	r.Get("/entries", h.List)
	r.Get("/entries/{id}", h.Get)
	r.Patch("/entries/{id}", h.Update)
	r.Delete("/entries/{id}", h.Delete)
	return r
}

// entriesRequestAs drives a request through the handler using the test-context
// caller shortcut (WithTestCallerID).
func entriesRequestAs(t *testing.T, db *sql.DB, method, path string, body any, userID string) *httptest.ResponseRecorder {
	t.Helper()
	r := entriesRouter(handler.NewEntriesForTest(db))

	var bodyBytes []byte
	if body != nil {
		bodyBytes, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	if userID != "" {
		req = req.WithContext(handler.WithTestCallerID(req.Context(), userID))
	}
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

// listContains reports whether List's response array includes the given entry id.
func listContains(t *testing.T, rr *httptest.ResponseRecorder, id string) bool {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 from List, got %d: %s", rr.Code, rr.Body.String())
	}
	var entries []handler.Entry
	if err := json.NewDecoder(rr.Body).Decode(&entries); err != nil {
		t.Fatalf("decode List response: %v", err)
	}
	for _, e := range entries {
		if e.ID == id {
			return true
		}
	}
	return false
}

// ── Case 1: owner sees their own private entry ──────────────────────────────────

func TestEntries_OwnerSeesOwnPrivateEntry(t *testing.T) {
	db := setupEntriesDB(t)
	seedEntry(t, db, "e1", "owner1", "private", "[]")

	if !listContains(t, entriesRequestAs(t, db, "GET", "/entries", nil, "owner1"), "e1") {
		t.Error("owner should see their own entry in List")
	}
	rr := entriesRequestAs(t, db, "GET", "/entries/e1", nil, "owner1")
	if rr.Code != http.StatusOK {
		t.Fatalf("owner Get: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ── Case 2: unrelated caller sees nothing ───────────────────────────────────────

func TestEntries_UnrelatedCallerBlocked(t *testing.T) {
	db := setupEntriesDB(t)
	seedEntry(t, db, "e1", "owner1", "private", "[]")

	if listContains(t, entriesRequestAs(t, db, "GET", "/entries", nil, "stranger"), "e1") {
		t.Error("unrelated caller must not see the entry in List")
	}
	rr := entriesRequestAs(t, db, "GET", "/entries/e1", nil, "stranger")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("unrelated Get: expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ── Case 3: entry_shares grant ──────────────────────────────────────────────────

func TestEntries_SharedCallerCanRead(t *testing.T) {
	db := setupEntriesDB(t)
	seedEntry(t, db, "e1", "owner1", "private", "[]")
	shareEntry(t, db, "e1", "friend")

	if !listContains(t, entriesRequestAs(t, db, "GET", "/entries", nil, "friend"), "e1") {
		t.Error("shared-with caller should see the entry in List")
	}
	rr := entriesRequestAs(t, db, "GET", "/entries/e1", nil, "friend")
	if rr.Code != http.StatusOK {
		t.Fatalf("shared Get: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ── Case 4: project membership (with negative twin) ─────────────────────────────

func TestEntries_ProjectMemberCanRead(t *testing.T) {
	db := setupEntriesDB(t)
	seedEntry(t, db, "e1", "owner1", "private", `["projA"]`)
	addProjectMember(t, db, "projA", "memberA")
	// Negative twin: member of an unrelated project must not see the entry.
	addProjectMember(t, db, "projB", "memberB")

	if !listContains(t, entriesRequestAs(t, db, "GET", "/entries", nil, "memberA"), "e1") {
		t.Error("member of the entry's project should see it in List")
	}
	if rr := entriesRequestAs(t, db, "GET", "/entries/e1", nil, "memberA"); rr.Code != http.StatusOK {
		t.Fatalf("project member Get: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	if listContains(t, entriesRequestAs(t, db, "GET", "/entries", nil, "memberB"), "e1") {
		t.Error("member of an unrelated project must NOT see the entry in List")
	}
	if rr := entriesRequestAs(t, db, "GET", "/entries/e1", nil, "memberB"); rr.Code != http.StatusNotFound {
		t.Fatalf("unrelated-project member Get: expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ── Case 5: admin gets no bypass (real JWT / real claims path) ───────────────────

func makeTestJWT(t *testing.T, secret []byte, sub, role string) string {
	t.Helper()
	claims := jwt.MapClaims{"sub": sub, "role": role, "active": true}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func TestEntries_AdminGetsNoBypass(t *testing.T) {
	db := setupEntriesDB(t)
	seedEntry(t, db, "e1", "owner1", "private", "[]")

	secret := []byte("test-secret")
	// Real handler + real claims path (not the WithTestCallerID shortcut): the
	// admin's role must not grant any read access to an entry they have no
	// ownership/share/membership relationship to.
	r := entriesRouter(handler.NewEntries(db, string(secret)))
	adminJWT := makeTestJWT(t, secret, "adminuser", "admin")

	// Get → 404
	getReq := httptest.NewRequest("GET", "/entries/e1", nil).
		WithContext(middleware.WithRawJWT(context.Background(), adminJWT))
	getRR := httptest.NewRecorder()
	r.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusNotFound {
		t.Fatalf("admin Get: expected 404 (no bypass), got %d: %s", getRR.Code, getRR.Body.String())
	}

	// List → absent
	listReq := httptest.NewRequest("GET", "/entries", nil).
		WithContext(middleware.WithRawJWT(context.Background(), adminJWT))
	listRR := httptest.NewRecorder()
	r.ServeHTTP(listRR, listReq)
	if listContains(t, listRR, "e1") {
		t.Error("admin must not see an unrelated entry in List (no bypass)")
	}
}

// ── Case 6: public visibility is not sufficient (the key regression lock) ───────

func TestEntries_PublicVisibilityNotSufficient(t *testing.T) {
	db := setupEntriesDB(t)
	// Public entry with no owner/share/project relationship to the caller.
	seedEntry(t, db, "e1", "owner1", "public", "[]")

	if listContains(t, entriesRequestAs(t, db, "GET", "/entries", nil, "stranger"), "e1") {
		t.Error("public entry must NOT be visible in List via the generic API")
	}
	rr := entriesRequestAs(t, db, "GET", "/entries/e1", nil, "stranger")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("public entry Get by stranger: expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ── SQL precedence guard ────────────────────────────────────────────────────────

// TestEntries_GetPrecedenceDoesNotLeakOwnedRows proves the outer parentheses in
// entryReadablePredicate. If the predicate were `A OR B OR C AND id=?`, SQLite's
// AND-before-OR would parse it as `A OR B OR (C AND id=?)`, so a Get for someone
// else's entry would still match any row the caller owns (A true) and return the
// WRONG entry. Correctly parenthesized, the caller owning a different entry gets
// a clean 404.
func TestEntries_GetPrecedenceDoesNotLeakOwnedRows(t *testing.T) {
	db := setupEntriesDB(t)
	seedEntry(t, db, "mine", "caller", "private", "[]")
	seedEntry(t, db, "theirs", "someone-else", "private", "[]")

	rr := entriesRequestAs(t, db, "GET", "/entries/theirs", nil, "caller")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for another user's entry, got %d: %s", rr.Code, rr.Body.String())
	}
	if body := rr.Body.String(); jsonHasEntryID(body, "mine") {
		t.Fatalf("precedence leak: Get for 'theirs' returned the caller's own 'mine' entry: %s", body)
	}
}

func jsonHasEntryID(body, id string) bool {
	var e handler.Entry
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		return false
	}
	return e.ID == id
}

// ── Case 7: Update/Delete are owner-only ────────────────────────────────────────

func TestEntries_UpdateOwnerOnly(t *testing.T) {
	db := setupEntriesDB(t)
	seedEntry(t, db, "e1", "owner1", "private", "[]")
	shareEntry(t, db, "e1", "friend") // read-access, must not imply write

	// Owner can update.
	rr := entriesRequestAs(t, db, "PATCH", "/entries/e1", map[string]any{"title": "New Title"}, "owner1")
	if rr.Code != http.StatusOK {
		t.Fatalf("owner Update: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var title string
	_ = db.QueryRow(`SELECT title FROM entries WHERE id = 'e1'`).Scan(&title)
	if title != "New Title" {
		t.Fatalf("owner Update did not persist: title=%q", title)
	}

	// Caller with a share grant (read access) cannot update.
	rr = entriesRequestAs(t, db, "PATCH", "/entries/e1", map[string]any{"title": "Hijacked"}, "friend")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("shared-caller Update: expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
	// Unrelated caller cannot update either.
	rr = entriesRequestAs(t, db, "PATCH", "/entries/e1", map[string]any{"title": "Hijacked"}, "stranger")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("stranger Update: expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
	_ = db.QueryRow(`SELECT title FROM entries WHERE id = 'e1'`).Scan(&title)
	if title != "New Title" {
		t.Fatalf("non-owner Update leaked through: title=%q", title)
	}
}

func TestEntries_DeleteOwnerOnly(t *testing.T) {
	db := setupEntriesDB(t)
	seedEntry(t, db, "e1", "owner1", "private", "[]")
	shareEntry(t, db, "e1", "friend")

	// Non-owner (with read share) cannot delete.
	rr := entriesRequestAs(t, db, "DELETE", "/entries/e1", nil, "friend")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("shared-caller Delete: expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM entries WHERE id = 'e1'`).Scan(&count)
	if count != 1 {
		t.Fatalf("non-owner Delete removed the entry: count=%d", count)
	}

	// Owner can delete.
	rr = entriesRequestAs(t, db, "DELETE", "/entries/e1", nil, "owner1")
	if rr.Code != http.StatusOK {
		t.Fatalf("owner Delete: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM entries WHERE id = 'e1'`).Scan(&count)
	if count != 0 {
		t.Fatalf("owner Delete did not remove the entry: count=%d", count)
	}
}

// ── Unauthenticated callers are rejected ────────────────────────────────────────

func TestEntries_UnauthenticatedRejected(t *testing.T) {
	db := setupEntriesDB(t)
	seedEntry(t, db, "e1", "owner1", "private", "[]")

	// No caller id in context and no JWT → 401 on every handler.
	for _, tc := range []struct {
		method, path string
	}{
		{"GET", "/entries"},
		{"GET", "/entries/e1"},
		{"PATCH", "/entries/e1"},
		{"DELETE", "/entries/e1"},
	} {
		rr := entriesRequestAs(t, db, tc.method, tc.path, map[string]any{"title": "x"}, "")
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: expected 401, got %d", tc.method, tc.path, rr.Code)
		}
	}
}
