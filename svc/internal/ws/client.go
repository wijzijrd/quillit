package ws

import (
	"time"

	"github.com/gorilla/websocket"
)

const (
	// sendBufferSize bounds a client's outbound queue. A consumer that can't keep
	// up (slow/dead) fills this and is evicted rather than blocking the room.
	sendBufferSize = 16

	// writeWait is how long a single write (including control frames) may take.
	writeWait = 10 * time.Second
	// pongWait is the read deadline; a pong (or any message) resets it.
	pongWait = 60 * time.Second
	// pingPeriod is how often the server sends a ping. Must be < pongWait.
	pingPeriod = 30 * time.Second
	// maxMessageSize caps an inbound frame at 8KB.
	maxMessageSize = 8192

	// minMsgInterval drops inbound messages arriving faster than this apart. This
	// guards against a buggy/malicious client, not real typing speed.
	minMsgInterval = 200 * time.Millisecond
)

// Client wraps a single WebSocket connection. Its send channel is written to by
// the Hub (broadcast) and drained by writePump. Lifecycle: the Hub owns the send
// channel (closes it exactly once, under the Hub mutex, when the client is
// removed); writePump owns conn.Close.
type Client struct {
	hub       *Hub
	projectID string
	conn      *websocket.Conn
	send      chan []byte

	// UserID is the authenticated caller behind this connection.
	UserID string

	// onMessage is invoked (in the read goroutine) for each accepted inbound
	// frame. The chat handler supplies this to dispatch chat/share_card messages.
	onMessage func(data []byte)

	lastMsg time.Time
}

// NewClient builds a Client. onMessage handles inbound frames (already rate
// limited and size capped). Call Start after registering it with the Hub.
func NewClient(hub *Hub, projectID, userID string, conn *websocket.Conn, onMessage func(data []byte)) *Client {
	return &Client{
		hub:       hub,
		projectID: projectID,
		conn:      conn,
		send:      make(chan []byte, sendBufferSize),
		UserID:    userID,
		onMessage: onMessage,
	}
}

// Start launches the read and write pumps. Each runs in its own goroutine and
// both exit (without leaking) when the connection closes or errors.
func (c *Client) Start() {
	go c.writePump()
	go c.readPump()
}

// readPump reads inbound frames, enforces the size cap and read deadline, keeps
// the connection alive via pong handling, and hands accepted frames to
// onMessage. It unregisters the client from the Hub on exit.
func (c *Client) readPump() {
	defer c.hub.Unregister(c.projectID, c)

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		// Blunt per-connection rate limit: silently drop bursts.
		now := time.Now()
		if now.Sub(c.lastMsg) < minMsgInterval {
			continue
		}
		c.lastMsg = now
		if c.onMessage != nil {
			c.onMessage(data)
		}
	}
}

// writePump drains the send channel to the socket and sends periodic pings. It
// owns conn.Close. When the Hub closes the send channel, it writes a normal WS
// close frame and returns.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel: this client is being removed.
				_ = c.conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
