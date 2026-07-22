package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	_ "modernc.org/sqlite"

	"github.com/quillit/svc/internal/handler"
	"github.com/quillit/svc/internal/middleware"
)

// This file proves the full product outcome the friends/sharing branch exists
// to deliver, end-to-end through the real HTTP handlers (not by seeding rows
// directly): two users become friends via FriendsHandler, one shares a private
// entry with the other via the now-friends-gated EntrySharesHandler.AddShares,
// and the recipient can actually read it back via EntriesHandler's
// entryReadablePredicate. Each of those three pieces already has isolated unit
// coverage (entries_test.go, entry_shares_test.go, friends_test.go); nothing
// previously exercised the seam between them.

const integrationJWTSecret = "integration-test-secret"

// integrationSchema is the union of the schema fragments the three handlers'
// own test files declare (setupEntriesDB + setupFriendsDB), since this test
// drives all three handlers against one shared in-memory DB.
const integrationSchema = `
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
CREATE TABLE friend_requests (
    id                  TEXT    PRIMARY KEY,
    requester_id        TEXT    NOT NULL,
    requester_username  TEXT    NOT NULL DEFAULT '',
    addressee_id        TEXT    NOT NULL,
    addressee_username  TEXT    NOT NULL DEFAULT '',
    status              TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','accepted')),
    created_at          INTEGER NOT NULL,
    accepted_at         INTEGER,
    CHECK (requester_id != addressee_id)
);
CREATE UNIQUE INDEX idx_friend_requests_pair
    ON friend_requests (MIN(requester_id, addressee_id), MAX(requester_id, addressee_id));
CREATE INDEX idx_friend_requests_addressee ON friend_requests(addressee_id, status);
CREATE INDEX idx_friend_requests_requester ON friend_requests(requester_id, status);
`

func setupIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(integrationSchema); err != nil {
		t.Fatal(err)
	}
	return db
}

// integrationRouter wires FriendsHandler, EntrySharesHandler, and
// EntriesHandler onto a single chi.Router over one shared DB, the way the
// real service composes them, so the test drives one HTTP surface instead of
// calling handler methods in isolation.
func integrationRouter(db *sql.DB) chi.Router {
	entriesH := handler.NewEntries(db, integrationJWTSecret)
	sharesH := handler.NewEntryShares(db, integrationJWTSecret, "http://unused.invalid")
	friendsH := handler.NewFriendsForTest(db, []byte(integrationJWTSecret))
	friendsH.SearchUsersFn = integrationStubSearchUsers

	r := chi.NewRouter()
	r.Post("/entries", entriesH.Create)
	r.Get("/entries", entriesH.List)
	r.Get("/entries/{id}", entriesH.Get)
	r.Post("/entries/{id}/shares", sharesH.AddShares)
	r.Post("/friends/requests", friendsH.SendRequest)
	r.Post("/friends/requests/{id}/accept", friendsH.AcceptRequest)
	r.Delete("/friends/requests/{id}", friendsH.DeleteRequest)
	return r
}

// integrationStubSearchUsers stands in for the auth-svc user-search proxy
// FriendsHandler.resolveOwnUsername calls, following the same
// "<id>@test.com" / "<id>name" convention friends_test.go's stubSearchUsers
// uses, so SendRequest's server-side identity resolution works without a live
// auth service.
func integrationStubSearchUsers(_ context.Context, _ string, query string) ([]handler.UserSearchResult, error) {
	id := strings.TrimSuffix(query, "@test.com")
	return []handler.UserSearchResult{{ID: id, Email: query, Username: id + "name"}}, nil
}

// integrationJWT signs a minimal claims set (sub, email, role, active) matching
// the shape entries_test.go's makeTestJWT / friends_test.go's makeFriendsJWT
// use elsewhere in this package.
func integrationJWT(t *testing.T, sub, email string) string {
	t.Helper()
	claims := jwt.MapClaims{"sub": sub, "email": email, "role": "user", "active": true}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(integrationJWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

// integrationRequest drives a request through r as the holder of jwtStr (a
// real signed JWT injected into context exactly as RequireSession would,
// mirroring middleware.WithRawJWT usage in entries_test.go/friends_test.go).
func integrationRequest(t *testing.T, r chi.Router, method, path string, body any, jwtStr string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	if jwtStr != "" {
		req = req.WithContext(middleware.WithRawJWT(req.Context(), jwtStr))
	}
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

// TestIntegration_AcceptShareRead_ThenUnfriendDoesNotRevoke drives the full
// accept -> share -> read chain through the real HTTP handlers, then proves
// this branch's confirmed design decision that unfriending afterward does not
// retroactively revoke an already-granted entry_shares row (matching the
// codebase's existing RevokeInvite-style "no retroactive revocation"
// convention).
func TestIntegration_AcceptShareRead_ThenUnfriendDoesNotRevoke(t *testing.T) {
	db := setupIntegrationDB(t)
	r := integrationRouter(db)

	const userA = "userA"
	const userB = "userB"
	jwtA := integrationJWT(t, userA, userA+"@test.com")
	jwtB := integrationJWT(t, userB, userB+"@test.com")

	// ── Step 1: A creates a private entry ───────────────────────────────────
	createRR := integrationRequest(t, r, http.MethodPost, "/entries", map[string]any{
		"title":      "A's Secret Note",
		"visibility": "private",
	}, jwtA)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("Create: expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}
	var entry handler.Entry
	if err := json.Unmarshal(createRR.Body.Bytes(), &entry); err != nil {
		t.Fatalf("decode created entry: %v", err)
	}
	if entry.OwnerUserID != userA {
		t.Fatalf("expected owner %q, got %q", userA, entry.OwnerUserID)
	}

	// Sanity: B cannot read it yet (not shared, not a project member).
	preShareRR := integrationRequest(t, r, http.MethodGet, "/entries/"+entry.ID, nil, jwtB)
	if preShareRR.Code != http.StatusNotFound {
		t.Fatalf("B pre-share Get: expected 404, got %d: %s", preShareRR.Code, preShareRR.Body.String())
	}

	// ── Step 2: A and B become friends (request + accept) ──────────────────
	sendRR := integrationRequest(t, r, http.MethodPost, "/friends/requests", map[string]string{
		"userId":   userB,
		"username": userB + "name",
	}, jwtA)
	if sendRR.Code != http.StatusCreated {
		t.Fatalf("SendRequest: expected 201, got %d: %s", sendRR.Code, sendRR.Body.String())
	}
	var fr handler.FriendRequest
	if err := json.Unmarshal(sendRR.Body.Bytes(), &fr); err != nil {
		t.Fatalf("decode friend request: %v", err)
	}

	acceptRR := integrationRequest(t, r, http.MethodPost, "/friends/requests/"+fr.ID+"/accept", nil, jwtB)
	if acceptRR.Code != http.StatusOK {
		t.Fatalf("AcceptRequest: expected 200, got %d: %s", acceptRR.Code, acceptRR.Body.String())
	}

	// ── Step 3: A shares the entry with B — now succeeds since they're friends ──
	shareRR := integrationRequest(t, r, http.MethodPost, "/entries/"+entry.ID+"/shares", map[string]any{
		"userIds": []string{userB},
	}, jwtA)
	if shareRR.Code != http.StatusOK {
		t.Fatalf("AddShares: expected 200 now that A and B are friends, got %d: %s", shareRR.Code, shareRR.Body.String())
	}

	// ── Step 4: B reads the entry via Get and List — the entry_shares grant ──
	// ── Task 5 created must be honored by Task 2's read predicate.          ──
	getRR := integrationRequest(t, r, http.MethodGet, "/entries/"+entry.ID, nil, jwtB)
	if getRR.Code != http.StatusOK {
		t.Fatalf("B Get after share: expected 200, got %d: %s", getRR.Code, getRR.Body.String())
	}
	var readEntry handler.Entry
	if err := json.Unmarshal(getRR.Body.Bytes(), &readEntry); err != nil {
		t.Fatalf("decode shared entry: %v", err)
	}
	if readEntry.ID != entry.ID {
		t.Fatalf("expected entry id %q, got %q", entry.ID, readEntry.ID)
	}

	listRR := integrationRequest(t, r, http.MethodGet, "/entries", nil, jwtB)
	if listRR.Code != http.StatusOK {
		t.Fatalf("B List after share: expected 200, got %d: %s", listRR.Code, listRR.Body.String())
	}
	var listed []handler.Entry
	if err := json.Unmarshal(listRR.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	found := false
	for _, e := range listed {
		if e.ID == entry.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected shared entry %q in B's List, got %+v", entry.ID, listed)
	}

	// ── Negative twin: unfriending does NOT revoke the existing share ──────────
	// This is a deliberate, confirmed design decision on this branch (see
	// entry_shares_test.go's TestAddShares_PreExistingShare_Grandfathered) —
	// locking it in here end-to-end so a future "fix" doesn't silently change it.
	unfriendRR := integrationRequest(t, r, http.MethodDelete, "/friends/requests/"+fr.ID, nil, jwtB)
	if unfriendRR.Code != http.StatusNoContent {
		t.Fatalf("DeleteRequest (unfriend): expected 204, got %d: %s", unfriendRR.Code, unfriendRR.Body.String())
	}

	postUnfriendGetRR := integrationRequest(t, r, http.MethodGet, "/entries/"+entry.ID, nil, jwtB)
	if postUnfriendGetRR.Code != http.StatusOK {
		t.Fatalf("B Get after unfriend: expected 200 (grandfathered share survives unfriend), got %d: %s",
			postUnfriendGetRR.Code, postUnfriendGetRR.Body.String())
	}

	postUnfriendListRR := integrationRequest(t, r, http.MethodGet, "/entries", nil, jwtB)
	if postUnfriendListRR.Code != http.StatusOK {
		t.Fatalf("B List after unfriend: expected 200, got %d: %s", postUnfriendListRR.Code, postUnfriendListRR.Body.String())
	}
	var listedAfterUnfriend []handler.Entry
	if err := json.Unmarshal(postUnfriendListRR.Body.Bytes(), &listedAfterUnfriend); err != nil {
		t.Fatalf("decode post-unfriend list: %v", err)
	}
	stillFound := false
	for _, e := range listedAfterUnfriend {
		if e.ID == entry.ID {
			stillFound = true
		}
	}
	if !stillFound {
		t.Fatalf("expected shared entry %q to still appear in B's List after unfriend (grandfathered), got %+v", entry.ID, listedAfterUnfriend)
	}

	// A new share attempt after unfriending must now be rejected again — proves
	// the grandfathering applies only to the pre-existing grant, not to future
	// sharing between the (no-longer-friends) pair.
	reshareRR := integrationRequest(t, r, http.MethodPost, "/entries/"+entry.ID+"/shares", map[string]any{
		"userIds": []string{userB},
	}, jwtA)
	if reshareRR.Code != http.StatusForbidden {
		t.Fatalf("AddShares after unfriend: expected 403 (no longer friends), got %d: %s", reshareRR.Code, reshareRR.Body.String())
	}
}
