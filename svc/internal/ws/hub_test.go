package ws

import (
	"sync"
	"testing"
)

// newTestClient builds a Client suitable for hub-only tests. The hub's
// broadcast/eviction/close paths only touch the send channel (never the
// connection), so a nil conn is safe as long as the pumps are not started.
func newTestClient(h *Hub, projectID string) *Client {
	return NewClient(h, projectID, "user", nil, nil)
}

// TestBroadcastReachesAllClients confirms a broadcast is delivered to every
// registered client in the room.
func TestBroadcastReachesAllClients(t *testing.T) {
	h := NewHub()
	const project = "p1"
	h.OpenRoom(project, "s1")

	c1 := newTestClient(h, project)
	c2 := newTestClient(h, project)
	h.Register(project, c1)
	h.Register(project, c2)

	payload := []byte(`{"type":"text","body":"hi"}`)
	h.Broadcast(project, payload)

	for i, c := range []*Client{c1, c2} {
		select {
		case got := <-c.send:
			if string(got) != string(payload) {
				t.Fatalf("client %d: got %q, want %q", i, got, payload)
			}
		default:
			t.Fatalf("client %d: expected a broadcast message, got none", i)
		}
	}
}

// TestBroadcastEvictsSlowConsumer confirms that a client whose send buffer is
// full is evicted (removed + send closed) without blocking delivery to the
// healthy client.
func TestBroadcastEvictsSlowConsumer(t *testing.T) {
	h := NewHub()
	const project = "p1"
	h.OpenRoom(project, "s1")

	slow := newTestClient(h, project)
	fast := newTestClient(h, project)
	h.Register(project, slow)
	h.Register(project, fast)

	// Saturate the slow client's send buffer so the next send would block.
	for i := 0; i < sendBufferSize; i++ {
		slow.send <- []byte("backlog")
	}

	payload := []byte(`{"type":"text","body":"hi"}`)
	h.Broadcast(project, payload)

	// The healthy client still receives the broadcast.
	select {
	case got := <-fast.send:
		if string(got) != string(payload) {
			t.Fatalf("fast client: got %q, want %q", got, payload)
		}
	default:
		t.Fatal("fast client: expected the broadcast, got none")
	}

	// The slow client was evicted: room now holds only the fast client.
	if n := h.RoomSize(project); n != 1 {
		t.Fatalf("room size after eviction: got %d, want 1", n)
	}
	if n := h.TotalConns(); n != 1 {
		t.Fatalf("total conns after eviction: got %d, want 1", n)
	}

	// The slow client's send channel was closed: after draining the backlog,
	// a receive reports the closed channel.
	for i := 0; i < sendBufferSize; i++ {
		<-slow.send
	}
	if _, ok := <-slow.send; ok {
		t.Fatal("slow client's send channel should be closed after eviction")
	}
}

// TestCloseRoomNotifiesAndRemoves confirms CloseRoom delivers the session_ended
// event, closes each client's send channel, and drops the room.
func TestCloseRoomNotifiesAndRemoves(t *testing.T) {
	h := NewHub()
	const project = "p1"
	h.OpenRoom(project, "s1")

	c := newTestClient(h, project)
	h.Register(project, c)

	h.CloseRoom(project)

	// First the session_ended event is queued...
	select {
	case got := <-c.send:
		if string(got) != string(sessionEndedPayload) {
			t.Fatalf("got %q, want %q", got, sessionEndedPayload)
		}
	default:
		t.Fatal("expected session_ended event before close")
	}
	// ...then the channel is closed.
	if _, ok := <-c.send; ok {
		t.Fatal("send channel should be closed after CloseRoom")
	}

	if n := h.RoomSize(project); n != 0 {
		t.Fatalf("room size after CloseRoom: got %d, want 0", n)
	}
	if n := h.TotalConns(); n != 0 {
		t.Fatalf("total conns after CloseRoom: got %d, want 0", n)
	}
}

// drainClosed reports whether c.send is closed, non-destructively consuming any
// payloads already queued ahead of the close. Safe only when no sender can still
// write to c.send (i.e. after all writers have been joined).
func drainClosed(c *Client) bool {
	for {
		select {
		case _, ok := <-c.send:
			if !ok {
				return true
			}
		default:
			return false
		}
	}
}

// TestOpenRoomAndRegisterIsAtomic confirms the combined call registers the
// client into the very room the hub stores (not a phantom, orphaned room) in a
// single step.
func TestOpenRoomAndRegisterIsAtomic(t *testing.T) {
	h := NewHub()
	const project = "p1"
	c := newTestClient(h, project)

	room := h.OpenRoomAndRegister(project, "s1", c)
	if room == nil {
		t.Fatal("expected a room, got nil")
	}
	if !room.clients[c] {
		t.Fatal("client was not registered into the returned room")
	}
	if room.SessionID != "s1" {
		t.Fatalf("room session id: got %q, want %q", room.SessionID, "s1")
	}

	// The returned room must be the one the hub actually tracks, otherwise a
	// later CloseRoom would close a different room and orphan this client.
	h.mu.Lock()
	stored := h.rooms[project]
	h.mu.Unlock()
	if stored != room {
		t.Fatal("returned room is not the room stored in the hub (orphaned)")
	}
	if n := h.RoomSize(project); n != 1 {
		t.Fatalf("room size: got %d, want 1", n)
	}
	if n := h.TotalConns(); n != 1 {
		t.Fatalf("total conns: got %d, want 1", n)
	}
}

// TestOpenRoomAndRegisterThenCloseRoomReachesClient is the anti-phantom
// property: a client added via the atomic call is always in the room a
// subsequent CloseRoom targets, so it receives session_ended and its send
// channel is closed — never left stranded in a room no CloseRoom is pending for.
func TestOpenRoomAndRegisterThenCloseRoomReachesClient(t *testing.T) {
	h := NewHub()
	const project = "p1"
	c := newTestClient(h, project)

	h.OpenRoomAndRegister(project, "s1", c)
	h.CloseRoom(project)

	select {
	case got, ok := <-c.send:
		if !ok {
			t.Fatal("expected session_ended before close, got a closed channel")
		}
		if string(got) != string(sessionEndedPayload) {
			t.Fatalf("got %q, want %q", got, sessionEndedPayload)
		}
	default:
		t.Fatal("expected session_ended event, got none (client orphaned)")
	}
	if _, ok := <-c.send; ok {
		t.Fatal("send channel should be closed after CloseRoom")
	}
	if n := h.RoomSize(project); n != 0 {
		t.Fatalf("room size after CloseRoom: got %d, want 0", n)
	}
}

// TestOpenRoomAndRegisterRaceWithCloseRoom races the combined open+register call
// against CloseRoom and asserts the client never lands in an inconsistent
// state. The single-lock atomicity guarantees the two operations are fully
// serialized, so exactly one of these holds afterward:
//   - CloseRoom won: the client's send channel is closed and its room is gone.
//   - Register won:  the client is present in a live room in the hub.
//
// The forbidden states the old two-call sequence could produce — a client
// registered into a room absent from the hub (orphan), or a closed client still
// present in a room (corruption) — must never occur. Run under -race.
func TestOpenRoomAndRegisterRaceWithCloseRoom(t *testing.T) {
	const iterations = 200
	for i := 0; i < iterations; i++ {
		h := NewHub()
		const project = "p1"
		// Seed a prior session so CloseRoom has a room to target even when it
		// wins the race to run first.
		h.OpenRoom(project, "s0")
		c := newTestClient(h, project)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			h.OpenRoomAndRegister(project, "s1", c)
		}()
		go func() {
			defer wg.Done()
			h.CloseRoom(project)
		}()
		wg.Wait()

		closed := drainClosed(c)
		size := h.RoomSize(project)
		conns := h.TotalConns()

		if closed {
			// CloseRoom saw the client: room and counters must be clear.
			if size != 0 {
				t.Fatalf("iter %d: client closed but room size=%d (corruption)", i, size)
			}
			if conns != 0 {
				t.Fatalf("iter %d: client closed but total conns=%d", i, conns)
			}
		} else {
			// Register won: client must be live and present in the hub's room.
			if size != 1 {
				t.Fatalf("iter %d: client open but room size=%d (orphaned)", i, size)
			}
			if conns != 1 {
				t.Fatalf("iter %d: client open but total conns=%d", i, conns)
			}
		}
	}
}

// TestUnregisterIsIdempotent confirms removing the same client twice does not
// panic (double-close of the send channel) — this happens in practice when an
// evicted client's readPump later also unregisters.
func TestUnregisterIsIdempotent(t *testing.T) {
	h := NewHub()
	const project = "p1"
	h.OpenRoom(project, "s1")

	c := newTestClient(h, project)
	h.Register(project, c)

	h.Unregister(project, c)
	h.Unregister(project, c) // must be a no-op, not a panic

	if n := h.TotalConns(); n != 0 {
		t.Fatalf("total conns: got %d, want 0", n)
	}
}
