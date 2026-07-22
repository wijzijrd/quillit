package handler_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/quillit/svc/internal/handler"
	"github.com/quillit/svc/internal/ws"
)

// setupChatWSDB builds an in-memory DB with the columns fetchResolved's
// entrySelect reads plus a chat_messages sink, seeded with one entry filed under
// proj1 and one filed under proj2.
func setupChatWSDB(t *testing.T) *sql.DB {
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
	// entryHere belongs to proj1 (the session's project); entryElsewhere belongs
	// to proj2 and must never be shareable into proj1's room.
	if _, err := db.Exec(
		`INSERT INTO entries (id,title,body,campaign_ids) VALUES ('entryHere','Local Lore','local body','["proj1"]')`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO entries (id,title,body,campaign_ids) VALUES ('entryElsewhere','Foreign Secret','secret body','["proj2"]')`,
	); err != nil {
		t.Fatal(err)
	}
	return db
}

// newChatWSFixture wires a ChatWSHandler over db plus a client registered in
// proj1's room, ready to receive broadcasts/rejections via TryRecv.
func newChatWSFixture(t *testing.T, db *sql.DB) (*handler.ChatWSHandler, *ws.Client) {
	t.Helper()
	hub := ws.NewHub()
	entries := handler.NewEntries(db, "test-secret")
	h := handler.NewChatWS(db, "test-secret", hub, entries, "")
	client := ws.NewClient(hub, "proj1", "user1", nil, nil)
	hub.OpenRoomAndRegister("proj1", "s1", client)
	return h, client
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
	raw, ok := client.TryRecv()
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
	if extra, ok := client.TryRecv(); ok {
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

	raw, ok := client.TryRecv()
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
