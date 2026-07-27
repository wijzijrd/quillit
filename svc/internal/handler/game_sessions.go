package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/quillit/svc/internal/middleware"
	"github.com/quillit/svc/internal/ws"
)

// GameSessionsHandler implements the Game Mode control-plane: starting/stopping
// a live session for a project, and reading back its chat history. The realtime
// (WebSocket) layer is built on top of this in a later task.
type GameSessionsHandler struct {
	db        *sql.DB
	jwtSecret []byte
	hub       *ws.Hub
}

func NewGameSessions(db *sql.DB, jwtSecret string, hub *ws.Hub) *GameSessionsHandler {
	return &GameSessionsHandler{db: db, jwtSecret: []byte(jwtSecret), hub: hub}
}

// NewGameSessionsForTest creates a handler that uses test context for caller ID.
func NewGameSessionsForTest(db *sql.DB) *GameSessionsHandler {
	return &GameSessionsHandler{db: db, jwtSecret: nil}
}

// NewGameSessionsWithHubForTest is like NewGameSessionsForTest but wires a real
// hub, so a test can exercise the Stop → hub.CloseRoom → client-disconnect path
// end-to-end (the nil-hub test constructor disables that side effect entirely).
func NewGameSessionsWithHubForTest(db *sql.DB, hub *ws.Hub) *GameSessionsHandler {
	return &GameSessionsHandler{db: db, jwtSecret: nil, hub: hub}
}

func (h *GameSessionsHandler) callerID(r *http.Request) (string, bool) {
	// Test helper: allow injecting caller ID without JWT.
	if id, ok := r.Context().Value(testCallerKey{}).(string); ok && id != "" {
		return id, true
	}
	mc, err := middleware.ClaimsFromContext(r.Context(), h.jwtSecret)
	if err != nil {
		return "", false
	}
	sub, _ := mc["sub"].(string)
	return sub, sub != ""
}

// isProjectMember reports whether userID is a member of projectID.
func (h *GameSessionsHandler) isProjectMember(r *http.Request, projectID, userID string) (bool, error) {
	var count int
	if err := h.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM project_members WHERE project_id = ? AND user_id = ?`,
		projectID, userID,
	).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// requireMember writes the appropriate error response and returns false if the
// caller is not authenticated or not a member of the given project.
func (h *GameSessionsHandler) requireMember(w http.ResponseWriter, r *http.Request, projectID string) (callerID string, ok bool) {
	callerID, authed := h.callerID(r)
	if !authed {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	member, err := h.isProjectMember(r, projectID, callerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return "", false
	}
	if !member {
		writeError(w, http.StatusForbidden, "not a project member")
		return "", false
	}
	return callerID, true
}

// ── Response types ────────────────────────────────────────────────────────────

type GameSession struct {
	ID        string  `json:"id"`
	ProjectID string  `json:"projectId"`
	Status    string  `json:"status"`
	StartedBy string  `json:"startedBy"`
	StartedAt int64   `json:"startedAt"`
	StoppedBy *string `json:"stoppedBy,omitempty"`
	StoppedAt *int64  `json:"stoppedAt,omitempty"`
}

type ChatMessage struct {
	ID        string  `json:"id"`
	SessionID string  `json:"sessionId"`
	ProjectID string  `json:"projectId"`
	SenderID  string  `json:"senderId"`
	Type      string  `json:"type"`
	Body      string  `json:"body"`
	EntryID   *string `json:"entryId,omitempty"`
	CardTitle string  `json:"cardTitle"`
	CardBody  string  `json:"cardBody"`
	CreatedAt int64   `json:"createdAt"`
}

const sessionSelect = `SELECT id, project_id, status, started_by, started_at, stopped_by, stopped_at FROM game_sessions`

func scanSession(row *sql.Row) (GameSession, error) {
	var s GameSession
	var stoppedBy sql.NullString
	var stoppedAt sql.NullInt64
	if err := row.Scan(&s.ID, &s.ProjectID, &s.Status, &s.StartedBy, &s.StartedAt, &stoppedBy, &stoppedAt); err != nil {
		return GameSession{}, err
	}
	if stoppedBy.Valid {
		s.StoppedBy = &stoppedBy.String
	}
	if stoppedAt.Valid {
		s.StoppedAt = &stoppedAt.Int64
	}
	return s, nil
}

// scanSessionRows is scanSession's *sql.Rows counterpart, for list queries.
func scanSessionRows(rows *sql.Rows) (GameSession, error) {
	var s GameSession
	var stoppedBy sql.NullString
	var stoppedAt sql.NullInt64
	if err := rows.Scan(&s.ID, &s.ProjectID, &s.Status, &s.StartedBy, &s.StartedAt, &stoppedBy, &stoppedAt); err != nil {
		return GameSession{}, err
	}
	if stoppedBy.Valid {
		s.StoppedBy = &stoppedBy.String
	}
	if stoppedAt.Valid {
		s.StoppedAt = &stoppedAt.Int64
	}
	return s, nil
}

func (h *GameSessionsHandler) fetchSessionByID(r *http.Request, id string) (GameSession, error) {
	return scanSession(h.db.QueryRowContext(r.Context(), sessionSelect+` WHERE id = ?`, id))
}

func (h *GameSessionsHandler) fetchRunningSession(r *http.Request, projectID string) (GameSession, error) {
	return scanSession(h.db.QueryRowContext(r.Context(), sessionSelect+` WHERE project_id = ? AND status = 'running'`, projectID))
}

// isUniqueConstraintErr reports whether err is a SQLite UNIQUE constraint violation.
func isUniqueConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// ── Start ─────────────────────────────────────────────────────────────────────

// Start godoc
// @Summary      Start a game session
// @Description  Starts a live session for the project. Any project member may
// @Description  start one. If a session is already running, returns it with 409.
// @Tags         game-sessions
// @Produce      json
// @Param        projectId  path  string  true  "Project ID"
// @Success      201  {object}  GameSession
// @Success      409  {object}  GameSession
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Router       /api/projects/{projectId}/session/start [post]
func (h *GameSessionsHandler) Start(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	callerID, ok := h.requireMember(w, r, projectID)
	if !ok {
		return
	}

	now := nowUnix()
	id := newID()
	_, err := h.db.ExecContext(r.Context(),
		`INSERT INTO game_sessions (id, project_id, status, started_by, started_at) VALUES (?, ?, 'running', ?, ?)`,
		id, projectID, callerID, now,
	)
	if err != nil {
		if isUniqueConstraintErr(err) {
			existing, ferr := h.fetchRunningSession(r, projectID)
			if ferr != nil {
				writeError(w, http.StatusInternalServerError, "db error")
				return
			}
			writeJSON(w, http.StatusConflict, existing)
			return
		}
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	session, err := h.fetchSessionByID(r, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

// ── Stop ──────────────────────────────────────────────────────────────────────

// Stop godoc
// @Summary      Stop the running game session
// @Description  Stops the currently running session for the project, if any.
// @Tags         game-sessions
// @Produce      json
// @Param        projectId  path  string  true  "Project ID"
// @Success      200  {object}  GameSession
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /api/projects/{projectId}/session/stop [post]
func (h *GameSessionsHandler) Stop(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	callerID, ok := h.requireMember(w, r, projectID)
	if !ok {
		return
	}

	now := nowUnix()
	// Atomically transition the running session to stopped and hand back its
	// own row in a single statement (SQLite RETURNING, supported by the
	// pinned modernc.org/sqlite driver). This avoids re-identifying the
	// stopped row afterward via project_id + stopped_by + stopped_at, which
	// is ambiguous when nowUnix()'s 1s resolution lets two stops by the same
	// caller land in the same second.
	row := h.db.QueryRowContext(r.Context(),
		`UPDATE game_sessions SET status = 'stopped', stopped_by = ?, stopped_at = ?
		 WHERE project_id = ? AND status = 'running'
		 RETURNING id, project_id, status, started_by, started_at, stopped_by, stopped_at`,
		callerID, now, projectID,
	)
	session, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "no active session")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	// Notify any connected clients that the session ended, then tear down the
	// room. Guarded because test handlers are constructed without a hub.
	if h.hub != nil {
		h.hub.CloseRoom(projectID)
	}

	writeJSON(w, http.StatusOK, session)
}

// ── Status ────────────────────────────────────────────────────────────────────

// Status godoc
// @Summary      Get the current session status for a project
// @Tags         game-sessions
// @Produce      json
// @Param        projectId  path  string  true  "Project ID"
// @Success      200  {object}  GameSession
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Router       /api/projects/{projectId}/session/status [get]
func (h *GameSessionsHandler) Status(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	if _, ok := h.requireMember(w, r, projectID); !ok {
		return
	}

	session, err := h.fetchRunningSession(r, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, session)
}

// ── ListMessages ──────────────────────────────────────────────────────────────

// ListMessages godoc
// @Summary      List chat history for a game session
// @Tags         game-sessions
// @Produce      json
// @Param        projectId  path  string  true  "Project ID"
// @Param        sessionId  path  string  true  "Session ID"
// @Success      200  {array}   ChatMessage
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /api/projects/{projectId}/session/{sessionId}/messages [get]
func (h *GameSessionsHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	sessionID := chi.URLParam(r, "sessionId")
	if _, ok := h.requireMember(w, r, projectID); !ok {
		return
	}

	var count int
	if err := h.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM game_sessions WHERE id = ? AND project_id = ?`, sessionID, projectID,
	).Scan(&count); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	if count == 0 {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	rows, err := h.db.QueryContext(r.Context(),
		`SELECT id, session_id, project_id, sender_id, type, body, entry_id, card_title, card_body, created_at
		 FROM chat_messages WHERE session_id = ? ORDER BY created_at ASC`, sessionID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	messages := []ChatMessage{}
	for rows.Next() {
		var m ChatMessage
		var entryID sql.NullString
		if err := rows.Scan(&m.ID, &m.SessionID, &m.ProjectID, &m.SenderID, &m.Type, &m.Body, &entryID, &m.CardTitle, &m.CardBody, &m.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "db error")
			return
		}
		if entryID.Valid {
			m.EntryID = &entryID.String
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, messages)
}

// ── ListSessions ──────────────────────────────────────────────────────────────

// ListSessions godoc
// @Summary      List past and current game sessions for a project
// @Description  Returns the project's sessions, newest first (max 100).
// @Tags         game-sessions
// @Produce      json
// @Param        projectId  path  string  true  "Project ID"
// @Success      200  {array}   GameSession
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Router       /api/projects/{projectId}/sessions [get]
func (h *GameSessionsHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	if _, ok := h.requireMember(w, r, projectID); !ok {
		return
	}

	rows, err := h.db.QueryContext(r.Context(),
		sessionSelect+` WHERE project_id = ? ORDER BY started_at DESC LIMIT 100`, projectID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	sessions := []GameSession{}
	for rows.Next() {
		s, err := scanSessionRows(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "db error")
			return
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}
