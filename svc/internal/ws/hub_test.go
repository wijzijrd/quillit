package ws

import (
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
