package ws

import (
	"encoding/json"
	"sync"
	"testing"
)

// newTestClient builds a Client suitable for hub-only tests. The hub's
// broadcast/eviction/close paths only touch the send channel (never the
// connection), so a nil conn is safe as long as the pumps are not started.
func newTestClient(h *Hub, projectID string) *Client {
	return newTestClientUser(h, projectID, "user")
}

// newTestClientUser is newTestClient with an explicit user ID, for presence
// tests that need distinct (or deliberately shared) users.
func newTestClientUser(h *Hub, projectID, userID string) *Client {
	return NewClient(h, projectID, userID, nil, nil)
}

// recvSkippingPresence pops the next non-presence frame queued for c, skipping
// any presence roster snapshots ahead of it. Fails the test if nothing else is
// queued.
func recvSkippingPresence(t *testing.T, c *Client) []byte {
	t.Helper()
	for {
		select {
		case got, ok := <-c.send:
			if !ok {
				t.Fatal("send channel closed while waiting for a frame")
			}
			if isPresenceFrame(got) {
				continue
			}
			return got
		default:
			t.Fatal("expected a queued frame, got none")
		}
	}
}

func isPresenceFrame(payload []byte) bool {
	return jsonType(payload) == "presence"
}

// jsonType extracts the "type" field of a frame payload.
func jsonType(payload []byte) string {
	var f struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(payload, &f)
	return f.Type
}

// drainPresence discards any presence frames currently queued for c.
func drainPresence(t *testing.T, c *Client) {
	t.Helper()
	for {
		select {
		case got, ok := <-c.send:
			if !ok {
				t.Fatal("send channel unexpectedly closed while draining presence")
			}
			if !isPresenceFrame(got) {
				t.Fatalf("expected only presence frames while draining, got %q", got)
			}
		default:
			return
		}
	}
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
		got := recvSkippingPresence(t, c)
		if string(got) != string(payload) {
			t.Fatalf("client %d: got %q, want %q", i, got, payload)
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
	// Registration queued presence frames; fill whatever capacity remains.
	for len(slow.send) < cap(slow.send) {
		slow.send <- []byte("backlog")
	}

	payload := []byte(`{"type":"text","body":"hi"}`)
	h.Broadcast(project, payload)

	// The healthy client still receives the broadcast.
	got := recvSkippingPresence(t, fast)
	if string(got) != string(payload) {
		t.Fatalf("fast client: got %q, want %q", got, payload)
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

	// First the session_ended event is queued (after registration's presence)...
	if got := recvSkippingPresence(t, c); string(got) != string(sessionEndedPayload) {
		t.Fatalf("got %q, want %q", got, sessionEndedPayload)
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

	if got := recvSkippingPresence(t, c); string(got) != string(sessionEndedPayload) {
		t.Fatalf("got %q, want %q", got, sessionEndedPayload)
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

// recvPresence pops the next queued frame for c and asserts it is a presence
// roster with exactly the given users.
func recvPresence(t *testing.T, c *Client, wantUsers []string) {
	t.Helper()
	select {
	case got, ok := <-c.send:
		if !ok {
			t.Fatal("send channel closed while expecting a presence frame")
		}
		var f struct {
			Type  string   `json:"type"`
			Users []string `json:"users"`
		}
		if err := json.Unmarshal(got, &f); err != nil {
			t.Fatalf("unmarshal presence frame: %v", err)
		}
		if f.Type != "presence" {
			t.Fatalf("frame type: got %q, want %q (payload %q)", f.Type, "presence", got)
		}
		if len(f.Users) != len(wantUsers) {
			t.Fatalf("presence users: got %v, want %v", f.Users, wantUsers)
		}
		for i := range wantUsers {
			if f.Users[i] != wantUsers[i] {
				t.Fatalf("presence users: got %v, want %v", f.Users, wantUsers)
			}
		}
	default:
		t.Fatal("expected a presence frame, got none")
	}
}

// TestPresenceRosterOnRegisterAndUnregister confirms every membership change
// broadcasts a full sorted roster snapshot: the joining client receives the
// roster as its first frame, existing clients see the join, and remaining
// clients see the leave.
func TestPresenceRosterOnRegisterAndUnregister(t *testing.T) {
	h := NewHub()
	const project = "p1"

	alice := newTestClientUser(h, project, "alice")
	h.OpenRoomAndRegister(project, "s1", alice)
	recvPresence(t, alice, []string{"alice"})

	bob := newTestClientUser(h, project, "bob")
	h.OpenRoomAndRegister(project, "s1", bob)
	recvPresence(t, alice, []string{"alice", "bob"})
	recvPresence(t, bob, []string{"alice", "bob"})

	h.Unregister(project, bob)
	recvPresence(t, alice, []string{"alice"})
}

// TestPresenceDeduplicatesUsers confirms one user with several connections
// (tabs) appears once in the roster, and remains present until the last of
// their connections leaves.
func TestPresenceDeduplicatesUsers(t *testing.T) {
	h := NewHub()
	const project = "p1"

	tab1 := newTestClientUser(h, project, "alice")
	tab2 := newTestClientUser(h, project, "alice")
	h.OpenRoomAndRegister(project, "s1", tab1)
	h.OpenRoomAndRegister(project, "s1", tab2)
	drainPresence(t, tab1)
	drainPresence(t, tab2)

	bob := newTestClientUser(h, project, "bob")
	h.OpenRoomAndRegister(project, "s1", bob)
	recvPresence(t, tab1, []string{"alice", "bob"})

	// One of alice's tabs closes: she is still present via the other.
	h.Unregister(project, tab2)
	drainPresence(t, tab1) // join+leave snapshots
	drainPresence(t, bob)

	h.Unregister(project, tab1)
	recvPresence(t, bob, []string{"bob"})
}

// TestCloseRoomSendsNoPresence confirms a closing room does not emit roster
// updates as its clients are torn down — they receive session_ended only.
func TestCloseRoomSendsNoPresence(t *testing.T) {
	h := NewHub()
	const project = "p1"

	alice := newTestClientUser(h, project, "alice")
	bob := newTestClientUser(h, project, "bob")
	h.OpenRoomAndRegister(project, "s1", alice)
	h.OpenRoomAndRegister(project, "s1", bob)
	drainPresence(t, alice)
	drainPresence(t, bob)

	h.CloseRoom(project)

	for _, c := range []*Client{alice, bob} {
		select {
		case got, ok := <-c.send:
			if !ok {
				t.Fatal("expected session_ended before close")
			}
			if string(got) != string(sessionEndedPayload) {
				t.Fatalf("got %q, want session_ended only (no presence)", got)
			}
		default:
			t.Fatal("expected session_ended event, got none")
		}
		if _, ok := <-c.send; ok {
			t.Fatal("send channel should be closed after CloseRoom")
		}
	}
}
