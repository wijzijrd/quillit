package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/quillit/svc/internal/middleware"
	"github.com/quillit/svc/internal/ws"
)

// ChatWSHandler upgrades a project's Game Mode chat to a WebSocket and bridges
// inbound frames to the shared in-process hub. Auth, membership, and the
// running-session check all happen before the HTTP handshake is upgraded.
type ChatWSHandler struct {
	db         *sql.DB
	jwtSecret  []byte
	hub        *ws.Hub
	entries    *EntriesHandler
	corsOrigin string
}

func NewChatWS(db *sql.DB, jwtSecret string, hub *ws.Hub, entries *EntriesHandler, corsOrigin string) *ChatWSHandler {
	return &ChatWSHandler{
		db:         db,
		jwtSecret:  []byte(jwtSecret),
		hub:        hub,
		entries:    entries,
		corsOrigin: corsOrigin,
	}
}

// inbound is the envelope for client → server frames. Only the two supported
// types are acted on; anything else is ignored.
type inbound struct {
	Type    string `json:"type"`
	Body    string `json:"body"`
	EntryID string `json:"entryId"`
}

// Serve validates the caller, confirms a running session, enforces connection
// caps, and (only then) upgrades the connection and starts the read/write pumps.
//
// Serve godoc
// @Summary      Open the Game Mode chat WebSocket for a project
// @Description  Upgrades to a WebSocket carrying live chat for the project's
// @Description  running session. Requires a session cookie and project membership.
// @Tags         game-sessions
// @Param        projectId  path  string  true  "Project ID"
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      409  {object}  ErrorResponse
// @Failure      503  {object}  ErrorResponse
// @Router       /api/projects/{projectId}/session/socket [get]
func (h *ChatWSHandler) Serve(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")

	// 1. Identity: the same-origin session cookie is validated by RequireSession
	//    upstream; extract the caller from the JWT it placed in context.
	mc, err := middleware.ClaimsFromContext(r.Context(), h.jwtSecret)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	callerID, _ := mc["sub"].(string)
	if callerID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// 2. Membership.
	var member int
	_ = h.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM project_members WHERE project_id = ? AND user_id = ?`,
		projectID, callerID,
	).Scan(&member)
	if member == 0 {
		writeError(w, http.StatusForbidden, "not a project member")
		return
	}

	// 3. A session must be running. Fail before the handshake, not after.
	var sessionID string
	err = h.db.QueryRowContext(r.Context(),
		`SELECT id FROM game_sessions WHERE project_id = ? AND status = 'running'`, projectID,
	).Scan(&sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusConflict, "no running session")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	// 4. Connection caps (blunt guardrails), before upgrade so we can send a
	//    plain HTTP 503 rather than a WS close.
	if h.hub.TotalConns() >= ws.ServerConnCap {
		writeError(w, http.StatusServiceUnavailable, "server connection limit reached")
		return
	}
	if h.hub.RoomSize(projectID) >= ws.RoomConnCap {
		writeError(w, http.StatusServiceUnavailable, "room connection limit reached")
		return
	}

	// 5. Upgrade with an Origin check tied to the same value CORS uses.
	upgrader := websocket.Upgrader{
		CheckOrigin: func(req *http.Request) bool {
			origin := req.Header.Get("Origin")
			// Non-browser clients omit Origin and aren't subject to CSRF; browsers
			// must match the configured app origin.
			return origin == "" || origin == h.corsOrigin
		},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade already wrote an error response on failure.
		log.Printf("chat_ws: upgrade failed for project %s: %v", projectID, err)
		return
	}

	// 6. Register and start pumps. The connection outlives this request, so the
	//    inbound handler must not use r.Context(). The onMessage closure captures
	//    client (declared first) so a share_card rejection can be sent to just
	//    this connection. Open-room-and-register is a single atomic hub call so a
	//    racing CloseRoom can't orphan this client in a phantom room.
	var client *ws.Client
	client = ws.NewClient(h.hub, projectID, callerID, conn, func(data []byte) {
		h.handleInbound(context.Background(), projectID, sessionID, callerID, client, data)
	})
	h.hub.OpenRoomAndRegister(projectID, sessionID, client)
	client.Start()
}

// shareCardRejectedPayload is sent to the requesting client alone (never the
// room) when a share_card names an entry that isn't part of this session's
// project. The frontend surfaces {"type":"error","message":...} to the sender.
var shareCardRejectedPayload = []byte(`{"type":"error","message":"entry not found in this project"}`)

// handleInbound parses one client frame and, for the two supported types,
// persists a chat_messages row and broadcasts the stored message to the room.
// client is the connection the frame arrived on, used to deliver per-sender
// rejections without leaking to the whole room.
func (h *ChatWSHandler) handleInbound(ctx context.Context, projectID, sessionID, senderID string, client *ws.Client, data []byte) {
	var in inbound
	if err := json.Unmarshal(data, &in); err != nil {
		return
	}

	switch in.Type {
	case "chat":
		if in.Body == "" {
			return
		}
		h.persistAndBroadcast(ctx, ChatMessage{
			SessionID: sessionID,
			ProjectID: projectID,
			SenderID:  senderID,
			Type:      "text",
			Body:      in.Body,
		})

	case "share_card":
		if in.EntryID == "" {
			return
		}
		entry, err := h.entries.fetchResolved(ctx, in.EntryID, senderID)
		if err != nil {
			return
		}
		// fetchResolved (shared with GET /api/entries/{id}) has no project
		// filter and returns any entry by id. Broadcasting fans a single-reader
		// gap out to the whole room, so scope it here: only share entries that
		// belong to this session's project (linkage lives in campaign_ids, the
		// JSON array of project ids an entry is filed under — see share.go).
		// A miss is rejected to the sender alone, not broadcast.
		if !entryInProject(entry, projectID) {
			h.hub.SendTo(client, shareCardRejectedPayload)
			return
		}
		entryID := in.EntryID
		h.persistAndBroadcast(ctx, ChatMessage{
			SessionID: sessionID,
			ProjectID: projectID,
			SenderID:  senderID,
			Type:      "note_card",
			EntryID:   &entryID,
			// Snapshot title/body server-side; the client never supplies these.
			CardTitle: entry.Title,
			CardBody:  entry.Body,
		})
	}
}

// entryInProject reports whether entry is filed under projectID. An entry's
// project membership is its campaign_ids JSON array (projects replaced the old
// campaigns; the column name is unchanged). A malformed/empty array means the
// entry belongs to no project and is never shareable into a room.
func entryInProject(entry Entry, projectID string) bool {
	var ids []string
	if err := json.Unmarshal(entry.CampaignIDs, &ids); err != nil {
		return false
	}
	for _, id := range ids {
		if id == projectID {
			return true
		}
	}
	return false
}

// persistAndBroadcast inserts msg (filling in id/createdAt), then broadcasts the
// stored row to the project's room.
func (h *ChatWSHandler) persistAndBroadcast(ctx context.Context, msg ChatMessage) {
	msg.ID = newID()
	msg.CreatedAt = nowUnix()

	var entryID any
	if msg.EntryID != nil {
		entryID = *msg.EntryID
	}
	_, err := h.db.ExecContext(ctx,
		`INSERT INTO chat_messages (id, session_id, project_id, sender_id, type, body, entry_id, card_title, card_body, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.SessionID, msg.ProjectID, msg.SenderID, msg.Type, msg.Body, entryID, msg.CardTitle, msg.CardBody, msg.CreatedAt,
	)
	if err != nil {
		log.Printf("chat_ws: insert message failed: %v", err)
		return
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.hub.Broadcast(msg.ProjectID, payload)
}
