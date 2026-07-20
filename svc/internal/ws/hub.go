package ws

import "sync"

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
	room.clients[c] = true
	h.total++
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
	for c := range room.clients {
		select {
		case c.send <- payload:
		default:
			// Full buffer: evict this one client, keep broadcasting to the rest.
			h.removeLocked(room, c)
		}
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
