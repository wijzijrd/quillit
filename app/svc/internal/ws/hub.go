package ws

import (
	"encoding/json"
	"sort"
	"sync"
)

// Connection caps. Enforced by the HTTP handler before upgrading.
const (
	// RoomConnCap bounds concurrent connections per project room.
	RoomConnCap = 20
	// ServerConnCap bounds concurrent connections across the whole process.
	ServerConnCap = 100
)

// sessionEndedPayload is broadcast to every client in a room when its session is
// stopped, just before their connections are closed. Kept as a fixed constant so
// the ws package stays free of the handler's response types.
var sessionEndedPayload = []byte(`{"type":"system","event":"session_ended"}`)

// presencePayloadLocked builds the {"type":"presence","users":[...]} frame
// listing room's distinct connected user IDs (a user with several tabs appears
// once), sorted for stable output. Caller must hold h.mu.
func presencePayloadLocked(room *Room) []byte {
	seen := make(map[string]bool, len(room.clients))
	users := make([]string, 0, len(room.clients))
	for c := range room.clients {
		if !seen[c.UserID] {
			seen[c.UserID] = true
			users = append(users, c.UserID)
		}
	}
	sort.Strings(users)
	payload, _ := json.Marshal(struct {
		Type  string   `json:"type"`
		Users []string `json:"users"`
	}{Type: "presence", Users: users})
	return payload
}

// broadcastPresenceLocked sends the current roster snapshot to every client in
// room. Best-effort: a full send buffer skips that client rather than evicting
// it — eviction would mutate room.clients mid-iteration via removeLocked, and
// the next presence change re-syncs anyone who missed one. Caller must hold
// h.mu.
func (h *Hub) broadcastPresenceLocked(room *Room) {
	if len(room.clients) == 0 {
		return
	}
	payload := presencePayloadLocked(room)
	for c := range room.clients {
		select {
		case c.send <- payload:
		default:
		}
	}
}

// Room holds the live clients for one project's game session.
type Room struct {
	SessionID string
	clients   map[*Client]bool
}

// Hub is the in-process, single-instance registry of project rooms. All access
// to the rooms map and each room's client set is guarded by mu. Multi-instance
// (horizontal) fan-out is explicitly out of scope for v1.
type Hub struct {
	mu    sync.Mutex
	rooms map[string]*Room
	// total tracks the process-wide connection count for ServerConnCap.
	total int
}

// NewHub creates an empty Hub.
func NewHub() *Hub {
	return &Hub{rooms: make(map[string]*Room)}
}

// OpenRoom returns the room for projectID, creating it if needed. The sessionID
// is refreshed so a room reused across restarts of a session tracks the current
// one.
func (h *Hub) OpenRoom(projectID, sessionID string) *Room {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.openRoomLocked(projectID, sessionID)
}

// openRoomLocked is OpenRoom's body. Caller must hold h.mu.
func (h *Hub) openRoomLocked(projectID, sessionID string) *Room {
	room := h.rooms[projectID]
	if room == nil {
		room = &Room{SessionID: sessionID, clients: make(map[*Client]bool)}
		h.rooms[projectID] = room
	} else {
		room.SessionID = sessionID
	}
	return room
}

// Register adds a client to its project room, creating the room if absent.
func (h *Hub) Register(projectID string, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.rooms[projectID]
	if room == nil {
		room = &Room{clients: make(map[*Client]bool)}
		h.rooms[projectID] = room
	}
	h.registerLocked(room, c)
	h.broadcastPresenceLocked(room)
}

// registerLocked adds c to room and bumps the process-wide count. Caller must
// hold h.mu.
func (h *Hub) registerLocked(room *Room, c *Client) {
	room.clients[c] = true
	h.total++
}

// OpenRoomAndRegister opens (or refreshes) the project's room and registers c
// into it under a single lock acquisition, then returns the room. Doing both
// atomically closes a race where a CloseRoom slipping between a separate
// OpenRoom and Register would delete the room and leave the just-registered
// client orphaned in a phantom room that never receives session_ended.
func (h *Hub) OpenRoomAndRegister(projectID, sessionID string, c *Client) *Room {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.openRoomLocked(projectID, sessionID)
	h.registerLocked(room, c)
	// The just-registered client receives the roster as its first queued frame
	// (delivered once its writePump starts); everyone else sees the join.
	h.broadcastPresenceLocked(room)
	return room
}

// SendTo delivers payload to a single client if it is still registered in its
// room, taking h.mu so the send races safely against concurrent eviction/close
// (which own closing c.send under the same lock). A full buffer evicts the
// client, matching Broadcast's slow-consumer policy.
func (h *Hub) SendTo(c *Client, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.rooms[c.projectID]
	if room == nil || !room.clients[c] {
		return
	}
	select {
	case c.send <- payload:
	default:
		h.removeLocked(room, c)
		h.broadcastPresenceLocked(room)
	}
}

// Unregister removes a client from its room and closes its send channel. Safe to
// call more than once for the same client (e.g. once from readPump exit and once
// from an earlier eviction): removal is idempotent because the client's presence
// in the room map is the single source of truth for whether send is still open.
func (h *Hub) Unregister(projectID string, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if room := h.rooms[projectID]; room != nil {
		h.removeLocked(room, c)
		h.broadcastPresenceLocked(room)
	}
}

// removeLocked deletes c from room and closes its send channel exactly once.
// Caller must hold h.mu.
func (h *Hub) removeLocked(room *Room, c *Client) {
	if _, ok := room.clients[c]; !ok {
		return
	}
	delete(room.clients, c)
	close(c.send)
	h.total--
}

// Broadcast sends payload to every client in the project's room. A client whose
// send buffer is full (slow/dead consumer) is evicted rather than blocking the
// broadcast to everyone else.
func (h *Hub) Broadcast(projectID string, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.rooms[projectID]
	if room == nil {
		return
	}
	evicted := false
	for c := range room.clients {
		select {
		case c.send <- payload:
		default:
			// Full buffer: evict this one client, keep broadcasting to the rest.
			h.removeLocked(room, c)
			evicted = true
		}
	}
	if evicted {
		h.broadcastPresenceLocked(room)
	}
}

// CloseRoom notifies every client in the project's room that the session ended,
// closes their connections (via a normal WS close, sent by each writePump when
// its send channel closes), and removes the room from the hub.
func (h *Hub) CloseRoom(projectID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.rooms[projectID]
	if room == nil {
		return
	}
	for c := range room.clients {
		// Best-effort delivery of the session_ended event before closing.
		select {
		case c.send <- sessionEndedPayload:
		default:
		}
		h.removeLocked(room, c)
	}
	delete(h.rooms, projectID)
}

// RoomSize returns the number of clients currently in the project's room.
func (h *Hub) RoomSize(projectID string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	if room := h.rooms[projectID]; room != nil {
		return len(room.clients)
	}
	return 0
}

// TotalConns returns the process-wide connection count.
func (h *Hub) TotalConns() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.total
}
