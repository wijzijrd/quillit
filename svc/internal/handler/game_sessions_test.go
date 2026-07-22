package handler_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/quillit/svc/internal/handler"
	"github.com/quillit/svc/internal/ws"
)

func setupGameSessionsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	schema := `
	CREATE TABLE projects (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, type TEXT NOT NULL DEFAULT 'campaign',
		created_by TEXT NOT NULL, created_at INTEGER NOT NULL
	);
	CREATE TABLE project_members (
		id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(id),
		user_id TEXT NOT NULL, role TEXT NOT NULL, joined_at INTEGER NOT NULL,
		username TEXT NOT NULL DEFAULT '',
		UNIQUE(project_id, user_id)
	);
	CREATE TABLE entries (
		id TEXT PRIMARY KEY, title TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE game_sessions (
		id          TEXT    PRIMARY KEY,
		project_id  TEXT    NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		status      TEXT    NOT NULL DEFAULT 'running',
		started_by  TEXT    NOT NULL,
		started_at  INTEGER NOT NULL,
		stopped_by  TEXT,
		stopped_at  INTEGER
	);
	CREATE INDEX idx_game_sessions_project ON game_sessions(project_id, status);
	CREATE UNIQUE INDEX idx_game_sessions_one_active
		ON game_sessions(project_id) WHERE status = 'running';
	CREATE TABLE chat_messages (
		id         TEXT    PRIMARY KEY,
		session_id TEXT    NOT NULL REFERENCES game_sessions(id) ON DELETE CASCADE,
		project_id TEXT    NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		sender_id  TEXT    NOT NULL,
		type       TEXT    NOT NULL DEFAULT 'text',
		body       TEXT    NOT NULL DEFAULT '',
		entry_id   TEXT    REFERENCES entries(id) ON DELETE SET NULL,
		card_title TEXT    NOT NULL DEFAULT '',
		card_body  TEXT    NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL
	);
	CREATE INDEX idx_chat_messages_session ON chat_messages(session_id, created_at);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	if _, err := db.Exec(`INSERT INTO projects VALUES ('proj1','TestCampaign','campaign','user1',?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_members VALUES ('m1','proj1','user1','gm',?,'gmuser')`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_members VALUES ('m2','proj1','user2','player',?,'playeruser')`, now); err != nil {
		t.Fatal(err)
	}
	return db
}

func gsRequest(t *testing.T, db *sql.DB, method, path string, body any, userID string) *httptest.ResponseRecorder {
	t.Helper()
	h := handler.NewGameSessionsForTest(db)

	r := chi.NewRouter()
	r.Post("/projects/{projectId}/session/start", h.Start)
	r.Post("/projects/{projectId}/session/stop", h.Stop)
	r.Get("/projects/{projectId}/session/status", h.Status)
	r.Get("/projects/{projectId}/session/{sessionId}/messages", h.ListMessages)
	r.Get("/projects/{projectId}/sessions", h.ListSessions)

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

// ── Start ─────────────────────────────────────────────────────────────────────

func TestGameSessionsStart_CreatesRunningSession(t *testing.T) {
	db := setupGameSessionsDB(t)
	rr := gsRequest(t, db, "POST", "/projects/proj1/session/start", nil, "user1")
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var s handler.GameSession
	if err := json.NewDecoder(rr.Body).Decode(&s); err != nil {
		t.Fatal(err)
	}
	if s.Status != "running" {
		t.Errorf("expected status=running, got %q", s.Status)
	}
	if s.StartedBy != "user1" {
		t.Errorf("expected startedBy=user1, got %q", s.StartedBy)
	}
	if s.ProjectID != "proj1" {
		t.Errorf("expected projectId=proj1, got %q", s.ProjectID)
	}
	if s.ID == "" {
		t.Error("expected non-empty id")
	}
}

func TestGameSessionsStart_SecondStartReturns409WithExisting(t *testing.T) {
	db := setupGameSessionsDB(t)
	rr1 := gsRequest(t, db, "POST", "/projects/proj1/session/start", nil, "user1")
	var first handler.GameSession
	json.NewDecoder(rr1.Body).Decode(&first)

	rr2 := gsRequest(t, db, "POST", "/projects/proj1/session/start", nil, "user2")
	if rr2.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr2.Code, rr2.Body.String())
	}
	var second handler.GameSession
	json.NewDecoder(rr2.Body).Decode(&second)
	if second.ID != first.ID {
		t.Errorf("expected existing session id %q, got %q", first.ID, second.ID)
	}

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM game_sessions WHERE project_id = 'proj1'`).Scan(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 session row, got %d", count)
	}
}

func TestGameSessionsStart_NonMemberRejected(t *testing.T) {
	db := setupGameSessionsDB(t)
	rr := gsRequest(t, db, "POST", "/projects/proj1/session/start", nil, "intruder")
	if rr.Code != http.StatusForbidden && rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 403 or 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ── Stop ──────────────────────────────────────────────────────────────────────

func TestGameSessionsStop_TransitionsStatus(t *testing.T) {
	db := setupGameSessionsDB(t)
	gsRequest(t, db, "POST", "/projects/proj1/session/start", nil, "user1")

	rr := gsRequest(t, db, "POST", "/projects/proj1/session/stop", nil, "user2")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var s handler.GameSession
	json.NewDecoder(rr.Body).Decode(&s)
	if s.Status != "stopped" {
		t.Errorf("expected status=stopped, got %q", s.Status)
	}
	if s.StoppedBy == nil || *s.StoppedBy != "user2" {
		t.Errorf("expected stoppedBy=user2, got %v", s.StoppedBy)
	}
	if s.StoppedAt == nil {
		t.Error("expected stoppedAt to be set")
	}
}

func TestGameSessionsStop_NoRunningSessionReturns404(t *testing.T) {
	db := setupGameSessionsDB(t)
	rr := gsRequest(t, db, "POST", "/projects/proj1/session/stop", nil, "user1")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGameSessionsStop_SecondSequentialStopReturns404(t *testing.T) {
	db := setupGameSessionsDB(t)
	gsRequest(t, db, "POST", "/projects/proj1/session/start", nil, "user1")

	rr1 := gsRequest(t, db, "POST", "/projects/proj1/session/stop", nil, "user1")
	if rr1.Code != http.StatusOK {
		t.Fatalf("expected first stop to return 200, got %d: %s", rr1.Code, rr1.Body.String())
	}

	rr2 := gsRequest(t, db, "POST", "/projects/proj1/session/stop", nil, "user2")
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("expected second stop to return 404, got %d: %s", rr2.Code, rr2.Body.String())
	}
}

func TestGameSessionsStop_ConcurrentStopsOnlyOneSucceeds(t *testing.T) {
	db := setupGameSessionsDB(t)
	gsRequest(t, db, "POST", "/projects/proj1/session/start", nil, "user1")

	var wg sync.WaitGroup
	codes := make([]int, 2)
	callers := []string{"user1", "user2"}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rr := gsRequest(t, db, "POST", "/projects/proj1/session/stop", nil, callers[i])
			codes[i] = rr.Code
		}(i)
	}
	wg.Wait()

	okCount, notFoundCount := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusOK:
			okCount++
		case http.StatusNotFound:
			notFoundCount++
		default:
			t.Errorf("unexpected status code %d", c)
		}
	}
	if okCount != 1 || notFoundCount != 1 {
		t.Fatalf("expected exactly one 200 and one 404 among concurrent stops, got codes=%v", codes)
	}

	var runningCount int
	db.QueryRow(`SELECT COUNT(*) FROM game_sessions WHERE project_id = 'proj1' AND status = 'running'`).Scan(&runningCount)
	if runningCount != 0 {
		t.Errorf("expected no running sessions left, got %d", runningCount)
	}
}

// TestGameSessionsStop_ReturnsOwnSessionNotAnotherStoppedOne guards against a
// regression where the post-UPDATE re-fetch identified the stopped row by
// project_id + stopped_by + stopped_at (all non-unique, and stopped_at has
// only 1s resolution). Here two full start/stop cycles are run for the same
// project by the same caller; if both stops happen to land in the same
// wall-clock second, a re-fetch keyed on that combination could return the
// FIRST session's data for the SECOND stop's response. Asserting the
// response id matches the session that was actually just started (and,
// separately, differs from the first session) catches that even when the
// two stops don't collide on the same second, and is the core correctness
// property regardless of timing.
func TestGameSessionsStop_ReturnsOwnSessionNotAnotherStoppedOne(t *testing.T) {
	db := setupGameSessionsDB(t)

	// First session: start and stop as user1.
	start1RR := gsRequest(t, db, "POST", "/projects/proj1/session/start", nil, "user1")
	var started1 handler.GameSession
	json.NewDecoder(start1RR.Body).Decode(&started1)

	stop1RR := gsRequest(t, db, "POST", "/projects/proj1/session/stop", nil, "user1")
	if stop1RR.Code != http.StatusOK {
		t.Fatalf("expected first stop to return 200, got %d: %s", stop1RR.Code, stop1RR.Body.String())
	}
	var stopped1 handler.GameSession
	json.NewDecoder(stop1RR.Body).Decode(&stopped1)
	if stopped1.ID != started1.ID {
		t.Fatalf("expected first stop to return session %q, got %q", started1.ID, stopped1.ID)
	}

	// Second session for the same project, stopped by the same caller. If
	// stopped_at happens to land in the same second as the first stop, the
	// old non-atomic re-fetch (project_id + stopped_by + stopped_at) could
	// return either row arbitrarily.
	start2RR := gsRequest(t, db, "POST", "/projects/proj1/session/start", nil, "user1")
	var started2 handler.GameSession
	json.NewDecoder(start2RR.Body).Decode(&started2)
	if started2.ID == started1.ID {
		t.Fatalf("expected second session to have a distinct id from the first")
	}

	stop2RR := gsRequest(t, db, "POST", "/projects/proj1/session/stop", nil, "user1")
	if stop2RR.Code != http.StatusOK {
		t.Fatalf("expected second stop to return 200, got %d: %s", stop2RR.Code, stop2RR.Body.String())
	}
	var stopped2 handler.GameSession
	json.NewDecoder(stop2RR.Body).Decode(&stopped2)

	if stopped2.ID != started2.ID {
		t.Errorf("expected second stop to return session %q (the one just stopped), got %q", started2.ID, stopped2.ID)
	}
	if stopped2.ID == stopped1.ID {
		t.Errorf("second stop returned the FIRST session's data (%q) instead of its own", stopped1.ID)
	}
	if stopped2.StartedAt != started2.StartedAt {
		t.Errorf("expected stopped session startedAt=%d (its own), got %d", started2.StartedAt, stopped2.StartedAt)
	}
}

func TestGameSessionsStop_NonMemberRejected(t *testing.T) {
	db := setupGameSessionsDB(t)
	gsRequest(t, db, "POST", "/projects/proj1/session/start", nil, "user1")
	rr := gsRequest(t, db, "POST", "/projects/proj1/session/stop", nil, "intruder")
	if rr.Code != http.StatusForbidden && rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 403 or 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestGameSessionsStop_ClosesRoomAndDisconnectsClients wires a REAL ws.Hub into
// the handler (unlike the nil-hub gsRequest path, where Stop's CloseRoom side
// effect is disabled by the `if h.hub != nil` guard) and proves the integration
// point end-to-end: stopping a session over the REST API actually tears down the
// live WebSocket clients in that project's room. hub_test.go covers CloseRoom's
// internals in isolation; this asserts the SAME behavior is reachable through the
// Stop handler — the two pieces working together, which nothing else exercised.
func TestGameSessionsStop_ClosesRoomAndDisconnectsClients(t *testing.T) {
	db := setupGameSessionsDB(t)
	hub := ws.NewHub()
	h := handler.NewGameSessionsWithHubForTest(db, hub)

	r := chi.NewRouter()
	r.Post("/projects/{projectId}/session/start", h.Start)
	r.Post("/projects/{projectId}/session/stop", h.Stop)

	callAs := func(method, path, userID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req = req.WithContext(handler.WithTestCallerID(req.Context(), userID))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	// Start a session so Stop has a running row to transition.
	startRR := callAs("POST", "/projects/proj1/session/start", "user1")
	if startRR.Code != http.StatusCreated {
		t.Fatalf("expected 201 from start, got %d: %s", startRR.Code, startRR.Body.String())
	}
	var started handler.GameSession
	if err := json.NewDecoder(startRR.Body).Decode(&started); err != nil {
		t.Fatal(err)
	}

	// Register a fake client into the room exactly as chat_ws.go's Serve does,
	// but with a nil conn so no real network I/O is needed (the hub's
	// close/eviction paths only touch the send channel). This is the same
	// technique hub_test.go's newTestClient uses.
	client := ws.NewClient(hub, "proj1", "user1", nil, nil)
	hub.OpenRoomAndRegister("proj1", started.ID, client)
	if n := hub.RoomSize("proj1"); n != 1 {
		t.Fatalf("expected room size 1 before stop, got %d", n)
	}

	// Stop the session via the REST handler.
	stopRR := callAs("POST", "/projects/proj1/session/stop", "user1")
	if stopRR.Code != http.StatusOK {
		t.Fatalf("expected 200 from stop, got %d: %s", stopRR.Code, stopRR.Body.String())
	}

	// (a) the connected client received the session_ended frame (skipping the
	// presence roster queued at registration).
	raw, ok := tryRecvSkippingPresence(client)
	if !ok {
		t.Fatal("expected a session_ended frame after Stop, got none")
	}
	var frame map[string]any
	if err := json.Unmarshal(raw, &frame); err != nil {
		t.Fatalf("session_ended frame is not JSON: %v", err)
	}
	if frame["type"] != "system" || frame["event"] != "session_ended" {
		t.Fatalf("expected {type:system,event:session_ended}, got %s", raw)
	}

	// (b) the room was torn down and the client removed — the same end state
	// CloseRoom's own unit test asserts, now reached through the Stop handler.
	if n := hub.RoomSize("proj1"); n != 0 {
		t.Fatalf("expected room torn down after Stop, got size %d", n)
	}
	if n := hub.TotalConns(); n != 0 {
		t.Fatalf("expected 0 total conns after Stop, got %d", n)
	}
}

// ── Status ────────────────────────────────────────────────────────────────────

func TestGameSessionsStatus_NoActiveSession(t *testing.T) {
	db := setupGameSessionsDB(t)
	rr := gsRequest(t, db, "GET", "/projects/proj1/session/status", nil, "user1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	json.NewDecoder(rr.Body).Decode(&body)
	if body["status"] != "stopped" {
		t.Errorf("expected status=stopped, got %v", body)
	}
}

func TestGameSessionsStatus_ActiveSession(t *testing.T) {
	db := setupGameSessionsDB(t)
	startRR := gsRequest(t, db, "POST", "/projects/proj1/session/start", nil, "user1")
	var started handler.GameSession
	json.NewDecoder(startRR.Body).Decode(&started)

	rr := gsRequest(t, db, "GET", "/projects/proj1/session/status", nil, "user2")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var s handler.GameSession
	json.NewDecoder(rr.Body).Decode(&s)
	if s.ID != started.ID {
		t.Errorf("expected id %q, got %q", started.ID, s.ID)
	}
	if s.Status != "running" {
		t.Errorf("expected status=running, got %q", s.Status)
	}
}

func TestGameSessionsStatus_NonMemberRejected(t *testing.T) {
	db := setupGameSessionsDB(t)
	rr := gsRequest(t, db, "GET", "/projects/proj1/session/status", nil, "intruder")
	if rr.Code != http.StatusForbidden && rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 403 or 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ── ListMessages ──────────────────────────────────────────────────────────────

func TestGameSessionsListMessages_EmptyForNewSession(t *testing.T) {
	db := setupGameSessionsDB(t)
	startRR := gsRequest(t, db, "POST", "/projects/proj1/session/start", nil, "user1")
	var started handler.GameSession
	json.NewDecoder(startRR.Body).Decode(&started)

	rr := gsRequest(t, db, "GET", fmt.Sprintf("/projects/proj1/session/%s/messages", started.ID), nil, "user1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var msgs []handler.ChatMessage
	if err := json.NewDecoder(rr.Body).Decode(&msgs); err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(msgs))
	}
}

func TestGameSessionsListMessages_OrderedByCreatedAt(t *testing.T) {
	db := setupGameSessionsDB(t)
	startRR := gsRequest(t, db, "POST", "/projects/proj1/session/start", nil, "user1")
	var started handler.GameSession
	json.NewDecoder(startRR.Body).Decode(&started)

	// Seed messages directly, out of chronological order.
	if _, err := db.Exec(
		`INSERT INTO chat_messages (id, session_id, project_id, sender_id, type, body, created_at) VALUES (?,?,?,?,?,?,?)`,
		"msg2", started.ID, "proj1", "user1", "text", "second", 200,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO chat_messages (id, session_id, project_id, sender_id, type, body, created_at) VALUES (?,?,?,?,?,?,?)`,
		"msg1", started.ID, "proj1", "user1", "text", "first", 100,
	); err != nil {
		t.Fatal(err)
	}

	rr := gsRequest(t, db, "GET", fmt.Sprintf("/projects/proj1/session/%s/messages", started.ID), nil, "user1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var msgs []handler.ChatMessage
	json.NewDecoder(rr.Body).Decode(&msgs)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].ID != "msg1" || msgs[1].ID != "msg2" {
		t.Errorf("expected order [msg1, msg2], got [%s, %s]", msgs[0].ID, msgs[1].ID)
	}
}

func TestGameSessionsListMessages_NonMemberRejected(t *testing.T) {
	db := setupGameSessionsDB(t)
	startRR := gsRequest(t, db, "POST", "/projects/proj1/session/start", nil, "user1")
	var started handler.GameSession
	json.NewDecoder(startRR.Body).Decode(&started)

	rr := gsRequest(t, db, "GET", fmt.Sprintf("/projects/proj1/session/%s/messages", started.ID), nil, "intruder")
	if rr.Code != http.StatusForbidden && rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 403 or 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ── ListSessions ──────────────────────────────────────────────────────────────

func TestGameSessionsListSessions_EmptyForNewProject(t *testing.T) {
	db := setupGameSessionsDB(t)
	rr := gsRequest(t, db, "GET", "/projects/proj1/sessions", nil, "user1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var sessions []handler.GameSession
	if err := json.NewDecoder(rr.Body).Decode(&sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestGameSessionsListSessions_NewestFirstMixedStatuses(t *testing.T) {
	db := setupGameSessionsDB(t)
	// Seed directly so started_at values are distinct and controlled: two
	// stopped sessions and one running, inserted out of order.
	seed := func(id string, startedAt int64, status string) {
		t.Helper()
		var stoppedBy any
		var stoppedAt any
		if status == "stopped" {
			stoppedBy, stoppedAt = "user1", startedAt+10
		}
		if _, err := db.Exec(
			`INSERT INTO game_sessions (id, project_id, status, started_by, started_at, stopped_by, stopped_at) VALUES (?,?,?,?,?,?,?)`,
			id, "proj1", status, "user1", startedAt, stoppedBy, stoppedAt,
		); err != nil {
			t.Fatal(err)
		}
	}
	seed("s-old", 100, "stopped")
	seed("s-new", 300, "running")
	seed("s-mid", 200, "stopped")

	rr := gsRequest(t, db, "GET", "/projects/proj1/sessions", nil, "user2")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var sessions []handler.GameSession
	if err := json.NewDecoder(rr.Body).Decode(&sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}
	wantOrder := []string{"s-new", "s-mid", "s-old"}
	for i, want := range wantOrder {
		if sessions[i].ID != want {
			t.Fatalf("expected order %v, got [%s %s %s]", wantOrder, sessions[0].ID, sessions[1].ID, sessions[2].ID)
		}
	}
	if sessions[0].Status != "running" {
		t.Errorf("expected newest session running, got %q", sessions[0].Status)
	}
	if sessions[1].StoppedBy == nil || sessions[1].StoppedAt == nil {
		t.Error("expected stopped session to carry stoppedBy/stoppedAt")
	}
}

func TestGameSessionsListSessions_NonMemberRejected(t *testing.T) {
	db := setupGameSessionsDB(t)
	rr := gsRequest(t, db, "GET", "/projects/proj1/sessions", nil, "intruder")
	if rr.Code != http.StatusForbidden && rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 403 or 401, got %d: %s", rr.Code, rr.Body.String())
	}
}
