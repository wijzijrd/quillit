package handler_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	_ "modernc.org/sqlite"

	v1 "github.com/quillit/gen/quillit/content/v1"
	"github.com/quillit/gen/quillit/content/v1/contentv1connect"

	"github.com/quillit/svc/internal/contentclient"
	"github.com/quillit/svc/internal/handler"
	"github.com/quillit/svc/internal/ws"
)

// setupChatWSDB builds an in-memory DB with just the chat_messages sink that
// persistAndBroadcast writes to. Entry data itself no longer lives in svc's
// DB — it's fetched from the content service via contentclient — so this
// schema no longer needs an entries table (see setupContentStub).
func setupChatWSDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	schema := `
	CREATE TABLE chat_messages (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		project_id TEXT NOT NULL,
		sender_id TEXT NOT NULL,
		type TEXT NOT NULL DEFAULT 'text',
		body TEXT NOT NULL DEFAULT '',
		entry_id TEXT,
		card_title TEXT NOT NULL DEFAULT '',
		card_body TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	return db
}

// stubContentInternal is a minimal contentv1connect.ContentInternalServiceHandler
// backed by an in-memory entries map, standing in for content's
// ContentInternalService.GetEntry — seeded with one entry filed under
// proj1 and one filed under proj2, mirroring the old DB-seeded fixture's
// entryHere/entryElsewhere but served over connectrpc the way
// contentclient.Client.Get expects (see contentclient_test.go for the same
// stub shape). Unknown ids return connect.CodeNotFound.
type stubContentInternal struct {
	entries map[string]*v1.GetEntryResponse
}

func (s stubContentInternal) GetEntry(_ context.Context, req *connect.Request[v1.GetEntryRequest]) (*connect.Response[v1.GetEntryResponse], error) {
	e, ok := s.entries[req.Msg.GetEntryId()]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errNotFoundStub)
	}
	return connect.NewResponse(e), nil
}

func (s stubContentInternal) NotifyProjectDeleted(_ context.Context, req *connect.Request[v1.NotifyProjectDeletedRequest]) (*connect.Response[v1.NotifyProjectDeletedResponse], error) {
	return connect.NewResponse(&v1.NotifyProjectDeletedResponse{ProjectId: req.Msg.GetProjectId()}), nil
}

var errNotFoundStub = errors.New("entry not found")

// setupContentStub starts an httptest.Server running a real
// ContentInternalService connect handler over stubContentInternal. The
// server is closed via t.Cleanup.
func setupContentStub(t *testing.T) *httptest.Server {
	t.Helper()
	stub := stubContentInternal{entries: map[string]*v1.GetEntryResponse{
		"entryHere": {
			Id: "entryHere", ProjectId: "proj1",
			Title: "Local Lore", Body: "local body",
		},
		"entryElsewhere": {
			Id: "entryElsewhere", ProjectId: "proj2",
			Title: "Foreign Secret", Body: "secret body",
		},
	}}
	path, h := contentv1connect.NewContentInternalServiceHandler(stub)
	mux := http.NewServeMux()
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newChatWSFixture wires a ChatWSHandler over db and a contentclient.Client
// pointed at a content-service stub, plus a client registered in proj1's
// room, ready to receive broadcasts/rejections via TryRecv.
func newChatWSFixture(t *testing.T, db *sql.DB) (*handler.ChatWSHandler, *ws.Client) {
	t.Helper()
	hub := ws.NewHub()
	content := contentclient.NewClient(setupContentStub(t).URL, "test-secret", nil)
	h := handler.NewChatWS(db, "test-secret", hub, content, "")
	client := ws.NewClient(hub, "proj1", "user1", nil, nil)
	hub.OpenRoomAndRegister("proj1", "s1", client)
	return h, client
}

// tryRecvSkippingPresence is client.TryRecv but discards the presence roster
// frames the hub queues on registration and membership changes, which are not
// what these tests assert on.
func tryRecvSkippingPresence(client *ws.Client) ([]byte, bool) {
	for {
		raw, ok := client.TryRecv()
		if !ok {
			return nil, false
		}
		var f struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &f) == nil && f.Type == "presence" {
			continue
		}
		return raw, true
	}
}

func countMessages(t *testing.T, db *sql.DB, entryID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM chat_messages WHERE entry_id = ?`, entryID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestShareCard_ForeignProjectEntryRejected is Finding 1: a share_card naming an
// entry filed under a different project must not be persisted or broadcast, and
// the requesting client alone gets an error frame.
func TestShareCard_ForeignProjectEntryRejected(t *testing.T) {
	db := setupChatWSDB(t)
	h, client := newChatWSFixture(t, db)

	h.HandleInboundForTest(context.Background(), "proj1", "s1", "user1", client,
		[]byte(`{"type":"share_card","entryId":"entryElsewhere"}`))

	// Not broadcast: nothing was persisted (broadcast only follows a successful
	// insert in persistAndBroadcast).
	if n := countMessages(t, db, "entryElsewhere"); n != 0 {
		t.Fatalf("foreign entry was persisted/broadcast: %d rows, want 0", n)
	}

	// Rejected: the sender receives an error frame addressed to it alone.
	raw, ok := tryRecvSkippingPresence(client)
	if !ok {
		t.Fatal("expected a rejection frame to the sender, got none")
	}
	var msg map[string]any
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("rejection frame is not JSON: %v", err)
	}
	if msg["type"] != "error" {
		t.Fatalf("expected type=error, got %v (frame=%s)", msg["type"], raw)
	}

	// And no note_card leaked to the room.
	if extra, ok := tryRecvSkippingPresence(client); ok {
		t.Fatalf("unexpected extra frame after rejection: %s", extra)
	}
}

// TestShareCard_SameProjectEntryBroadcast is the positive control: an entry
// filed under the session's project is persisted as a note_card and broadcast to
// the room.
func TestShareCard_SameProjectEntryBroadcast(t *testing.T) {
	db := setupChatWSDB(t)
	h, client := newChatWSFixture(t, db)

	h.HandleInboundForTest(context.Background(), "proj1", "s1", "user1", client,
		[]byte(`{"type":"share_card","entryId":"entryHere"}`))

	if n := countMessages(t, db, "entryHere"); n != 1 {
		t.Fatalf("expected the shared entry to be persisted once, got %d rows", n)
	}

	raw, ok := tryRecvSkippingPresence(client)
	if !ok {
		t.Fatal("expected a note_card broadcast, got none")
	}
	var msg handler.ChatMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("broadcast frame is not a ChatMessage: %v", err)
	}
	if msg.Type != "note_card" {
		t.Fatalf("expected type=note_card, got %q", msg.Type)
	}
	if msg.CardTitle != "Local Lore" {
		t.Fatalf("expected server-snapshotted card title, got %q", msg.CardTitle)
	}
}
