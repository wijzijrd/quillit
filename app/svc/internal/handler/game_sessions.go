package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/quillit/svc/internal/db/sqlc"
	"github.com/quillit/svc/internal/middleware"
	"github.com/quillit/svc/internal/ws"
)

// GameSessionsHandler implements the Game Mode control-plane: starting/stopping
// a live session for a project, and reading back its chat history. The realtime
// (WebSocket) layer is built on top of this in a later task.
type GameSessionsHandler struct {
	q         *sqlc.Queries
	jwtSecret []byte
	hub       *ws.Hub
}

func NewGameSessions(db *sql.DB, jwtSecret string, hub *ws.Hub) *GameSessionsHandler {
	return &GameSessionsHandler{q: sqlc.New(db), jwtSecret: []byte(jwtSecret), hub: hub}
}

// NewGameSessionsForTest creates a handler that uses test context for caller ID.
func NewGameSessionsForTest(db *sql.DB) *GameSessionsHandler {
	return &GameSessionsHandler{q: sqlc.New(db), jwtSecret: nil}
}

// NewGameSessionsWithHubForTest is like NewGameSessionsForTest but wires a real
// hub, so a test can exercise the Stop → hub.CloseRoom → client-disconnect path
// end-to-end (the nil-hub test constructor disables that side effect entirely).
func NewGameSessionsWithHubForTest(db *sql.DB, hub *ws.Hub) *GameSessionsHandler {
	return &GameSessionsHandler{q: sqlc.New(db), jwtSecret: nil, hub: hub}
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
	count, err := h.q.CountProjectMembership(r.Context(), sqlc.CountProjectMembershipParams{
		ProjectID: projectID,
		UserID:    userID,
	})
	if err != nil {
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

// toGameSession converts a generated sqlc row into the handler's GameSession
// response type, unwrapping the nullable stopped_by/stopped_at columns into
// the pointer fields the JSON response uses.
func toGameSession(g sqlc.GameSession) GameSession {
	s := GameSession{
		ID:        g.ID,
		ProjectID: g.ProjectID,
		Status:    g.Status,
		StartedBy: g.StartedBy,
		StartedAt: g.StartedAt,
	}
	if g.StoppedBy.Valid {
		s.StoppedBy = &g.StoppedBy.String
	}
	if g.StoppedAt.Valid {
		s.StoppedAt = &g.StoppedAt.Int64
	}
	return s
}

func (h *GameSessionsHandler) fetchSessionByID(r *http.Request, id string) (GameSession, error) {
	g, err := h.q.GetGameSession(r.Context(), id)
	if err != nil {
		return GameSession{}, err
	}
	return toGameSession(g), nil
}

func (h *GameSessionsHandler) fetchRunningSession(r *http.Request, projectID string) (GameSession, error) {
	g, err := h.q.GetRunningGameSession(r.Context(), projectID)
	if err != nil {
		return GameSession{}, err
	}
	return toGameSession(g), nil
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
	err := h.q.InsertGameSession(r.Context(), sqlc.InsertGameSessionParams{
		ID:        id,
		ProjectID: projectID,
		StartedBy: callerID,
		StartedAt: now,
	})
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
	g, err := h.q.StopGameSession(r.Context(), sqlc.StopGameSessionParams{
		StoppedBy: sql.NullString{String: callerID, Valid: true},
		StoppedAt: sql.NullInt64{Int64: now, Valid: true},
		ProjectID: projectID,
	})
	session := toGameSession(g)
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

	count, err := h.q.CountGameSessionByIDAndProject(r.Context(), sqlc.CountGameSessionByIDAndProjectParams{
		ID:        sessionID,
		ProjectID: projectID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	if count == 0 {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	rows, err := h.q.ListChatMessages(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	messages := []ChatMessage{}
	for _, row := range rows {
		m := ChatMessage{
			ID:        row.ID,
			SessionID: row.SessionID,
			ProjectID: row.ProjectID,
			SenderID:  row.SenderID,
			Type:      row.Type,
			Body:      row.Body,
			CardTitle: row.CardTitle,
			CardBody:  row.CardBody,
			CreatedAt: row.CreatedAt,
		}
		if row.EntryID.Valid {
			m.EntryID = &row.EntryID.String
		}
		messages = append(messages, m)
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

	rows, err := h.q.ListGameSessionsForProject(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	sessions := []GameSession{}
	for _, row := range rows {
		sessions = append(sessions, toGameSession(row))
	}
	writeJSON(w, http.StatusOK, sessions)
}
