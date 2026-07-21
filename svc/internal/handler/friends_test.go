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

// setupFriendsDB builds an in-memory DB with the exact friend_requests schema
// from db.toV8, including the pair-normalized unique index, so tests exercise
// the real DB-level constraint rather than a hand-simplified stand-in.
func setupFriendsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	schema := `
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
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	return db
}

func frRouter(db *sql.DB) chi.Router {
	h := handler.NewFriendsForTest(db)
	r := chi.NewRouter()
	r.Post("/friends/requests", h.SendRequest)
	r.Get("/friends/requests/incoming", h.ListIncoming)
	r.Get("/friends/requests/outgoing", h.ListOutgoing)
	r.Post("/friends/requests/{id}/accept", h.AcceptRequest)
	r.Delete("/friends/requests/{id}", h.DeleteRequest)
	r.Get("/friends", h.ListFriends)
	return r
}

func frRequest(t *testing.T, db *sql.DB, method, path string, body any, userID string) *httptest.ResponseRecorder {
	t.Helper()
	r := frRouter(db)

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

func decodeFR(t *testing.T, rr *httptest.ResponseRecorder) handler.FriendRequest {
	t.Helper()
	var fr handler.FriendRequest
	if err := json.Unmarshal(rr.Body.Bytes(), &fr); err != nil {
		t.Fatalf("decode FriendRequest: %v (body=%s)", err, rr.Body.String())
	}
	return fr
}

func decodeFRList(t *testing.T, rr *httptest.ResponseRecorder) []handler.FriendRequest {
	t.Helper()
	var out []handler.FriendRequest
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode []FriendRequest: %v (body=%s)", err, rr.Body.String())
	}
	return out
}

func decodeFriends(t *testing.T, rr *httptest.ResponseRecorder) []handler.Friend {
	t.Helper()
	var out []handler.Friend
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode []Friend: %v (body=%s)", err, rr.Body.String())
	}
	return out
}

func sendRequest(t *testing.T, db *sql.DB, from, toUserID, toUsername string) *httptest.ResponseRecorder {
	t.Helper()
	return frRequest(t, db, http.MethodPost, "/friends/requests", map[string]string{
		"userId":            toUserID,
		"username":          toUsername,
		"requesterUsername": from + "name",
	}, from)
}

// ── Send + list ───────────────────────────────────────────────────────────────

func TestSendRequest_AppearsInOutgoingAndIncoming(t *testing.T) {
	db := setupFriendsDB(t)

	rr := sendRequest(t, db, "user1", "user2", "user2name")
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	fr := decodeFR(t, rr)
	if fr.Status != "pending" {
		t.Errorf("expected status=pending, got %q", fr.Status)
	}
	if fr.RequesterID != "user1" || fr.AddresseeID != "user2" {
		t.Errorf("unexpected requester/addressee: %+v", fr)
	}

	out := decodeFRList(t, frRequest(t, db, http.MethodGet, "/friends/requests/outgoing", nil, "user1"))
	if len(out) != 1 || out[0].ID != fr.ID {
		t.Errorf("expected request in user1's outgoing list, got %+v", out)
	}

	in := decodeFRList(t, frRequest(t, db, http.MethodGet, "/friends/requests/incoming", nil, "user2"))
	if len(in) != 1 || in[0].ID != fr.ID {
		t.Errorf("expected request in user2's incoming list, got %+v", in)
	}
}

func TestSendRequest_SelfRejected(t *testing.T) {
	db := setupFriendsDB(t)
	rr := sendRequest(t, db, "user1", "user1", "user1name")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSendRequest_DuplicatePendingRejected(t *testing.T) {
	db := setupFriendsDB(t)

	rr := sendRequest(t, db, "user1", "user2", "user2name")
	if rr.Code != http.StatusCreated {
		t.Fatalf("first send: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// Same direction.
	rr2 := sendRequest(t, db, "user1", "user2", "user2name")
	if rr2.Code != http.StatusConflict {
		t.Errorf("same-direction duplicate: expected 409, got %d: %s", rr2.Code, rr2.Body.String())
	}

	// Reverse direction.
	rr3 := sendRequest(t, db, "user2", "user1", "user1name")
	if rr3.Code != http.StatusConflict {
		t.Errorf("reverse-direction duplicate: expected 409, got %d: %s", rr3.Code, rr3.Body.String())
	}
}

func TestSendRequest_AlreadyFriendsRejected(t *testing.T) {
	db := setupFriendsDB(t)

	fr := decodeFR(t, sendRequest(t, db, "user1", "user2", "user2name"))
	acceptRR := frRequest(t, db, http.MethodPost, "/friends/requests/"+fr.ID+"/accept", nil, "user2")
	if acceptRR.Code != http.StatusOK {
		t.Fatalf("accept: expected 200, got %d: %s", acceptRR.Code, acceptRR.Body.String())
	}

	rr := sendRequest(t, db, "user1", "user2", "user2name")
	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409 already-friends, got %d: %s", rr.Code, rr.Body.String())
	}
	rrReverse := sendRequest(t, db, "user2", "user1", "user1name")
	if rrReverse.Code != http.StatusConflict {
		t.Errorf("expected 409 already-friends (reverse), got %d: %s", rrReverse.Code, rrReverse.Body.String())
	}
}

// ── Accept ────────────────────────────────────────────────────────────────────

func TestAcceptRequest_OnlyAddresseeCanAccept(t *testing.T) {
	db := setupFriendsDB(t)
	fr := decodeFR(t, sendRequest(t, db, "user1", "user2", "user2name"))

	rr := frRequest(t, db, http.MethodPost, "/friends/requests/"+fr.ID+"/accept", nil, "user1")
	if rr.Code != http.StatusForbidden {
		t.Errorf("requester accepting own request: expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAcceptRequest_FlipsStatusAndAppearsInFriendsList(t *testing.T) {
	db := setupFriendsDB(t)
	fr := decodeFR(t, sendRequest(t, db, "user1", "user2", "user2name"))

	rr := frRequest(t, db, http.MethodPost, "/friends/requests/"+fr.ID+"/accept", nil, "user2")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	updated := decodeFR(t, rr)
	if updated.Status != "accepted" || updated.AcceptedAt == nil {
		t.Errorf("expected accepted status with acceptedAt set, got %+v", updated)
	}

	f1 := decodeFriends(t, frRequest(t, db, http.MethodGet, "/friends", nil, "user1"))
	if len(f1) != 1 || f1[0].UserID != "user2" || f1[0].Username != "user2name" {
		t.Errorf("expected user1's friends list to contain user2, got %+v", f1)
	}
	f2 := decodeFriends(t, frRequest(t, db, http.MethodGet, "/friends", nil, "user2"))
	if len(f2) != 1 || f2[0].UserID != "user1" {
		t.Errorf("expected user2's friends list to contain user1, got %+v", f2)
	}
}

func TestAcceptRequest_AlreadyAcceptedReturns409(t *testing.T) {
	db := setupFriendsDB(t)
	fr := decodeFR(t, sendRequest(t, db, "user1", "user2", "user2name"))
	frRequest(t, db, http.MethodPost, "/friends/requests/"+fr.ID+"/accept", nil, "user2")

	rr := frRequest(t, db, http.MethodPost, "/friends/requests/"+fr.ID+"/accept", nil, "user2")
	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAcceptRequest_NonexistentReturns404(t *testing.T) {
	db := setupFriendsDB(t)
	rr := frRequest(t, db, http.MethodPost, "/friends/requests/doesnotexist/accept", nil, "user2")
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ── Decline / cancel / unfriend ─────────────────────────────────────────────────

func TestDeleteRequest_DeclineByAddressee(t *testing.T) {
	db := setupFriendsDB(t)
	fr := decodeFR(t, sendRequest(t, db, "user1", "user2", "user2name"))

	rr := frRequest(t, db, http.MethodDelete, "/friends/requests/"+fr.ID, nil, "user2")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	out := decodeFRList(t, frRequest(t, db, http.MethodGet, "/friends/requests/outgoing", nil, "user1"))
	if len(out) != 0 {
		t.Errorf("expected empty outgoing list after decline, got %+v", out)
	}
	in := decodeFRList(t, frRequest(t, db, http.MethodGet, "/friends/requests/incoming", nil, "user2"))
	if len(in) != 0 {
		t.Errorf("expected empty incoming list after decline, got %+v", in)
	}
}

func TestDeleteRequest_CancelByRequester(t *testing.T) {
	db := setupFriendsDB(t)
	fr := decodeFR(t, sendRequest(t, db, "user1", "user2", "user2name"))

	rr := frRequest(t, db, http.MethodDelete, "/friends/requests/"+fr.ID, nil, "user1")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	out := decodeFRList(t, frRequest(t, db, http.MethodGet, "/friends/requests/outgoing", nil, "user1"))
	if len(out) != 0 {
		t.Errorf("expected empty outgoing list after cancel, got %+v", out)
	}
	in := decodeFRList(t, frRequest(t, db, http.MethodGet, "/friends/requests/incoming", nil, "user2"))
	if len(in) != 0 {
		t.Errorf("expected empty incoming list after cancel, got %+v", in)
	}
}

func TestDeleteRequest_UnfriendByEitherParty_AllowsFreshRequestAfter(t *testing.T) {
	db := setupFriendsDB(t)
	fr := decodeFR(t, sendRequest(t, db, "user1", "user2", "user2name"))
	frRequest(t, db, http.MethodPost, "/friends/requests/"+fr.ID+"/accept", nil, "user2")

	rr := frRequest(t, db, http.MethodDelete, "/friends/requests/"+fr.ID, nil, "user2")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	f1 := decodeFriends(t, frRequest(t, db, http.MethodGet, "/friends", nil, "user1"))
	if len(f1) != 0 {
		t.Errorf("expected user1's friends list empty after unfriend, got %+v", f1)
	}
	f2 := decodeFriends(t, frRequest(t, db, http.MethodGet, "/friends", nil, "user2"))
	if len(f2) != 0 {
		t.Errorf("expected user2's friends list empty after unfriend, got %+v", f2)
	}

	// Bonus: a fresh request between the same pair must succeed post-unfriend —
	// proves the pair-uniqueness index doesn't wrongly latch on the deleted row.
	rr2 := sendRequest(t, db, "user1", "user2", "user2name")
	if rr2.Code != http.StatusCreated {
		t.Errorf("expected fresh request to succeed after unfriend, got %d: %s", rr2.Code, rr2.Body.String())
	}
}

func TestDeleteRequest_UnrelatedCallerRejected(t *testing.T) {
	db := setupFriendsDB(t)
	fr := decodeFR(t, sendRequest(t, db, "user1", "user2", "user2name"))

	rr := frRequest(t, db, http.MethodDelete, "/friends/requests/"+fr.ID, nil, "user3")
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAcceptRequest_UnrelatedCallerRejected(t *testing.T) {
	db := setupFriendsDB(t)
	fr := decodeFR(t, sendRequest(t, db, "user1", "user2", "user2name"))

	rr := frRequest(t, db, http.MethodPost, "/friends/requests/"+fr.ID+"/accept", nil, "user3")
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteRequest_NonexistentReturns404(t *testing.T) {
	db := setupFriendsDB(t)
	rr := frRequest(t, db, http.MethodDelete, "/friends/requests/doesnotexist", nil, "user1")
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ── Pair-uniqueness DB-level constraint ─────────────────────────────────────────

// TestFriendRequestsPairUniqueness_DBLevelConstraint bypasses the handler
// entirely and inserts directly via database/sql, proving the unique index
// itself (not just the handler's SELECT-before-INSERT pre-check) rejects a
// second row for the same normalized pair — in either id ordering.
func TestFriendRequestsPairUniqueness_DBLevelConstraint(t *testing.T) {
	db := setupFriendsDB(t)

	_, err := db.Exec(
		`INSERT INTO friend_requests (id, requester_id, requester_username, addressee_id, addressee_username, status, created_at)
		 VALUES (?, ?, ?, ?, ?, 'pending', ?)`,
		"req-1", "user1", "user1name", "user2", "user2name", 1000,
	)
	if err != nil {
		t.Fatalf("first insert should succeed: %v", err)
	}

	// Same direction, different id.
	_, err = db.Exec(
		`INSERT INTO friend_requests (id, requester_id, requester_username, addressee_id, addressee_username, status, created_at)
		 VALUES (?, ?, ?, ?, ?, 'pending', ?)`,
		"req-2", "user1", "user1name", "user2", "user2name", 1001,
	)
	if err == nil {
		t.Fatal("expected second insert (same direction) to fail with a UNIQUE constraint violation, but it succeeded")
	}
	t.Logf("same-direction duplicate insert correctly rejected: %v", err)

	// Reverse direction, different id — must also collide because the index is
	// built on MIN/MAX of the pair, not the raw column order.
	_, err = db.Exec(
		`INSERT INTO friend_requests (id, requester_id, requester_username, addressee_id, addressee_username, status, created_at)
		 VALUES (?, ?, ?, ?, ?, 'pending', ?)`,
		"req-3", "user2", "user2name", "user1", "user1name", 1002,
	)
	if err == nil {
		t.Fatal("expected reverse-direction insert to fail with a UNIQUE constraint violation, but it succeeded")
	}
	t.Logf("reverse-direction duplicate insert correctly rejected: %v", err)

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM friend_requests`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 row to have survived, got %d", count)
	}
}
