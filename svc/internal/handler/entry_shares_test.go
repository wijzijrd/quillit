package handler_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/quillit/svc/internal/handler"
)

// setupEntrySharesDB builds an in-memory DB with the subset of schema
// AddShares/ListShares/RemoveShare and the isFriend helper touch: entries,
// entry_shares, and friend_requests (trimmed to the columns isFriend's query
// reads — see friends.go).
func setupEntrySharesDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	schema := `
	CREATE TABLE entries (
		id            TEXT PRIMARY KEY,
		title         TEXT NOT NULL DEFAULT '',
		owner_user_id TEXT
	);
	CREATE TABLE entry_shares (
		id        TEXT PRIMARY KEY,
		entry_id  TEXT NOT NULL,
		user_id   TEXT NOT NULL,
		shared_by TEXT NOT NULL,
		shared_at INTEGER NOT NULL,
		UNIQUE(entry_id, user_id)
	);
	CREATE TABLE friend_requests (
		id           TEXT PRIMARY KEY,
		requester_id TEXT NOT NULL,
		addressee_id TEXT NOT NULL,
		status       TEXT NOT NULL DEFAULT 'pending',
		created_at   INTEGER NOT NULL DEFAULT 0
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	return db
}

func esSeedEntry(t *testing.T, db *sql.DB, id, owner string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO entries (id, title, owner_user_id) VALUES (?, ?, ?)`,
		id, "Title "+id, owner,
	); err != nil {
		t.Fatal(err)
	}
}

// esMakeFriends inserts an accepted friend_requests row directly, so a and b
// are friends per isFriend's query.
func esMakeFriends(t *testing.T, db *sql.DB, a, b string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO friend_requests (id, requester_id, addressee_id, status, created_at) VALUES (?, ?, ?, 'accepted', 1)`,
		"fr-"+a+"-"+b, a, b,
	); err != nil {
		t.Fatal(err)
	}
}

// esSeedLegacyShare inserts an entry_shares row directly, bypassing AddShares
// entirely — simulating a share made before friends-only enforcement existed.
func esSeedLegacyShare(t *testing.T, db *sql.DB, entryID, userID, sharedBy string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO entry_shares (id, entry_id, user_id, shared_by, shared_at) VALUES (?, ?, ?, ?, 1)`,
		"legacy-"+entryID+"-"+userID, entryID, userID, sharedBy,
	); err != nil {
		t.Fatal(err)
	}
}

func esCountShares(t *testing.T, db *sql.DB, entryID, userID string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM entry_shares WHERE entry_id = ? AND user_id = ?`, entryID, userID,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func esCountAllShares(t *testing.T, db *sql.DB, entryID string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM entry_shares WHERE entry_id = ?`, entryID,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func esRouter(db *sql.DB) chi.Router {
	h := handler.NewEntryShares(db, "entry-shares-test-secret", "http://unused.invalid")
	r := chi.NewRouter()
	r.Get("/entries/{id}/shares", h.ListShares)
	r.Post("/entries/{id}/shares", h.AddShares)
	r.Delete("/entries/{id}/shares/{userId}", h.RemoveShare)
	return r
}

func esRequest(t *testing.T, db *sql.DB, method, path string, body any, userID string) *httptest.ResponseRecorder {
	t.Helper()
	r := esRouter(db)

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

// ── Friends-only enforcement ────────────────────────────────────────────────

func TestAddShares_WithFriend_Succeeds(t *testing.T) {
	db := setupEntrySharesDB(t)
	esSeedEntry(t, db, "e1", "owner")
	esMakeFriends(t, db, "owner", "friend1")

	rr := esRequest(t, db, http.MethodPost, "/entries/e1/shares", map[string]any{
		"userIds": []string{"friend1"},
	}, "owner")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := esCountShares(t, db, "e1", "friend1"); got != 1 {
		t.Errorf("expected 1 entry_shares row for friend1, got %d", got)
	}
}

// TestAddShares_NonFriend_Rejected proves a share attempt targeting a
// non-friend is rejected with 403 and, critically, creates NO entry_shares
// row — verified via a direct query, not just the HTTP status.
func TestAddShares_NonFriend_Rejected(t *testing.T) {
	db := setupEntrySharesDB(t)
	esSeedEntry(t, db, "e1", "owner")
	// No friend_requests row between owner and stranger at all.

	rr := esRequest(t, db, http.MethodPost, "/entries/e1/shares", map[string]any{
		"userIds": []string{"stranger"},
	}, "owner")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := esCountShares(t, db, "e1", "stranger"); got != 0 {
		t.Errorf("expected NO entry_shares row for stranger after rejection, got %d", got)
	}
}

// TestAddShares_MixedBatch_RejectsWholeBatch proves the fail-closed,
// all-or-nothing behavior: a batch containing one friend and one non-friend
// is rejected in full (403), and NEITHER row is created — proving the
// friend-in-the-batch doesn't get partially shared while the non-friend is
// silently dropped.
func TestAddShares_MixedBatch_RejectsWholeBatch(t *testing.T) {
	db := setupEntrySharesDB(t)
	esSeedEntry(t, db, "e1", "owner")
	esMakeFriends(t, db, "owner", "friend1")
	// stranger is deliberately not a friend.

	rr := esRequest(t, db, http.MethodPost, "/entries/e1/shares", map[string]any{
		"userIds": []string{"friend1", "stranger"},
	}, "owner")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for mixed friend/non-friend batch, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := esCountAllShares(t, db, "e1"); got != 0 {
		t.Errorf("expected NO entry_shares rows at all after mixed-batch rejection (fail-closed on whole batch), got %d", got)
	}
}

// TestAddShares_PreExistingShare_Grandfathered proves shares created before
// friends-only enforcement existed are unaffected: ListShares still returns
// them and RemoveShare still works, even though the two parties are not
// friends. No migration/backfill/revocation is expected or added.
func TestAddShares_PreExistingShare_Grandfathered(t *testing.T) {
	db := setupEntrySharesDB(t)
	esSeedEntry(t, db, "e1", "owner")
	// legacyuser and owner are NOT friends — simulating a share made back
	// when sharing was unrestricted.
	esSeedLegacyShare(t, db, "e1", "legacyuser", "owner")

	listRR := esRequest(t, db, http.MethodGet, "/entries/e1/shares", nil, "owner")
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200 listing shares, got %d: %s", listRR.Code, listRR.Body.String())
	}
	var shares []handler.EntryShare
	if err := json.Unmarshal(listRR.Body.Bytes(), &shares); err != nil {
		t.Fatalf("decode shares: %v", err)
	}
	found := false
	for _, s := range shares {
		if s.UserID == "legacyuser" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected pre-existing legacy share to still appear in ListShares, got %+v", shares)
	}

	removeRR := esRequest(t, db, http.MethodDelete, "/entries/e1/shares/legacyuser", nil, "owner")
	if removeRR.Code != http.StatusOK {
		t.Fatalf("expected 200 removing legacy share, got %d: %s", removeRR.Code, removeRR.Body.String())
	}
	if got := esCountShares(t, db, "e1", "legacyuser"); got != 0 {
		t.Errorf("expected legacy share removed, still found %d rows", got)
	}
}
