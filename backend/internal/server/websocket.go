package server

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"together/backend/internal/realtime"
)

const (
	// wsSendBuffer bounds a connection's outbound queue before it is treated as
	// slow and dropped by the hub.
	wsSendBuffer = 32
	// wsMaxMessageBytes caps an inbound frame.
	wsMaxMessageBytes = 4096
	// wsPongWait is how long we wait for a pong before assuming the peer died.
	wsPongWait = 60 * time.Second
	// wsPingPeriod must be less than wsPongWait.
	wsPingPeriod = 30 * time.Second
	// wsWriteWait bounds a single write.
	wsWriteWait = 10 * time.Second
)

// handleWebSocket authenticates the session, enforces the app Origin, upgrades
// the connection and registers it with the hub. No application messages are
// delivered yet; the connection is kept healthy with ping/pong.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	u, status := s.authenticate(r)
	if status != 0 {
		if status == http.StatusUnauthorized {
			unauthorized(w)
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	// Strict Origin match, mirroring csrfProtect. (The Upgrader also checks.)
	if r.Header.Get("Origin") != s.cfg.AppOrigin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	conn, err := s.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade has already written an error response.
		return
	}

	client := realtime.NewClient(u.ID, wsSendBuffer)
	s.hub.Register(client)

	go s.wsWritePump(conn, client)
	s.wsReadPump(conn, client)
}

// wsReadPump drains inbound frames (currently discarded) and keeps the read
// deadline fresh via pong replies. It owns connection teardown.
func (s *Server) wsReadPump(conn *websocket.Conn, client *realtime.Client) {
	defer func() {
		s.hub.Unregister(client)
		_ = conn.Close()
	}()

	conn.SetReadLimit(wsMaxMessageBytes)
	_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		// No inbound message handling yet.
	}
}

// wsWritePump writes queued frames and periodic pings. It exits when the client
// is retired (by the hub or the read pump) or a write fails.
func (s *Server) wsWritePump(conn *websocket.Conn, client *realtime.Client) {
	ticker := time.NewTicker(wsPingPeriod)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-client.Send():
			_ = conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if !ok {
				_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-client.Closed():
			_ = conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		}
	}
}
