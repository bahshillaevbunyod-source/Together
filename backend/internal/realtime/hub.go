// Package realtime provides a small, concurrency-safe hub that tracks live
// client connections per user. It is transport-agnostic: the hub only moves
// bytes into per-client buffered channels and never blocks on a slow client.
package realtime

import "sync"

// Client is one live connection belonging to a user. Outbound frames are queued
// on a buffered channel; the transport layer drains Send() and writes them.
type Client struct {
	userID    string
	send      chan []byte
	closeOnce sync.Once
	closed    chan struct{}
}

// NewClient builds a client with an outbound buffer of the given size.
func NewClient(userID string, buffer int) *Client {
	if buffer < 1 {
		buffer = 1
	}
	return &Client{
		userID: userID,
		send:   make(chan []byte, buffer),
		closed: make(chan struct{}),
	}
}

// UserID returns the client's owner.
func (c *Client) UserID() string { return c.userID }

// Send is the outbound queue the transport writer drains.
func (c *Client) Send() <-chan []byte { return c.send }

// Closed is closed exactly once when the client is retired.
func (c *Client) Closed() <-chan struct{} { return c.closed }

// close retires the client exactly once.
func (c *Client) close() { c.closeOnce.Do(func() { close(c.closed) }) }

// Hub tracks connected clients keyed by user id. All methods are safe for
// concurrent use.
type Hub struct {
	mu      sync.Mutex
	clients map[string]map[*Client]struct{}
}

// NewHub builds an empty hub.
func NewHub() *Hub {
	return &Hub{clients: make(map[string]map[*Client]struct{})}
}

// Register adds a client. A user may hold several connections at once.
func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.clients[c.userID]
	if set == nil {
		set = make(map[*Client]struct{})
		h.clients[c.userID] = set
	}
	set[c] = struct{}{}
}

// Unregister removes a client and retires it. Safe to call more than once.
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	h.removeLocked(c)
	h.mu.Unlock()
	c.close()
}

// removeLocked drops a client from its user's set; caller holds the lock.
func (h *Hub) removeLocked(c *Client) {
	set := h.clients[c.userID]
	if set == nil {
		return
	}
	if _, ok := set[c]; ok {
		delete(set, c)
		if len(set) == 0 {
			delete(h.clients, c.userID)
		}
	}
}

// ConnectionCount returns how many live connections a user currently holds.
func (h *Hub) ConnectionCount(userID string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients[userID])
}

// TotalConnections returns the number of live connections across all users.
func (h *Hub) TotalConnections() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, set := range h.clients {
		n += len(set)
	}
	return n
}

// SendToUser delivers msg to every connection of userID without ever blocking.
// A client whose buffer is full is treated as too slow: it is removed and
// retired so it can never stall the others or the caller.
func (h *Hub) SendToUser(userID string, msg []byte) {
	h.mu.Lock()
	var slow []*Client
	for c := range h.clients[userID] {
		select {
		case c.send <- msg:
		default:
			slow = append(slow, c)
		}
	}
	for _, c := range slow {
		h.removeLocked(c)
	}
	h.mu.Unlock()

	for _, c := range slow {
		c.close()
	}
}
