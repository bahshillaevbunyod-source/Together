package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"together/backend/internal/call"
	"together/backend/internal/realtime"
)

const (
	// wsSendBuffer bounds a connection's outbound queue before it is treated as
	// slow and dropped by the hub.
	wsSendBuffer = 32
	// wsMaxMessageBytes caps an inbound frame.
	wsMaxMessageBytes = 64 << 10
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
	s.wsReadPump(conn, client, u.ID)
}

// wsReadPump drains inbound frames (currently discarded) and keeps the read
// deadline fresh via pong replies. It owns connection teardown.
func (s *Server) wsReadPump(conn *websocket.Conn, client *realtime.Client, userID string) {
	defer func() {
		s.hub.Unregister(client)
		if s.hub.ConnectionCount(userID) == 0 {
			for _, ended := range s.calls.EndByUser(userID) {
				other := ended.CallerID
				if other == userID {
					other = ended.CalleeID
				}
				s.relayCall(other, "call.end", map[string]any{"callId": ended.ID})
			}
		}
		_ = conn.Close()
	}()

	conn.SetReadLimit(wsMaxMessageBytes)
	_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		s.handleCallFrame(userID, raw)
	}
}

func (s *Server) handleCallFrame(userID string, raw []byte) {
	var env struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if len(raw) > wsMaxMessageBytes || json.Unmarshal(raw, &env) != nil || !strings.HasPrefix(env.Type, "call.") {
		return
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(env.Data, &fields) != nil {
		return
	}
	id, ok := validStringField(fields, "callId", 128)
	if !ok {
		return
	}
	e, ok := s.calls.Get(id)
	if !ok || (userID != e.CallerID && userID != e.CalleeID) {
		return
	}
	other := e.CallerID
	if other == userID {
		other = e.CalleeID
	}
	switch env.Type {
	case "call.offer":
		if userID != e.CallerID || e.State != call.Created {
			return
		}
		sdp, ok := validStringField(fields, "sdp", 64<<10)
		if !ok {
			return
		}
		if s.hub.ConnectionCount(other) == 0 {
			if _, err := s.calls.End(id, userID); err == nil {
				s.relayCall(userID, "call.reject", map[string]any{"callId": id})
			}
			return
		}
		if _, err := s.calls.BeginRinging(id, userID); err != nil {
			return
		}
		caller, err := s.users.GetByID(context.Background(), userID)
		if err != nil {
			return
		}
		s.relayCall(other, env.Type, map[string]any{"callId": id, "conversationId": e.ConversationID, "callType": e.Type, "caller": callPeer(caller), "sdp": sdp})
	case "call.answer":
		if userID != e.CalleeID || e.State != call.Ringing {
			return
		}
		sdp, ok := validStringField(fields, "sdp", 64<<10)
		if !ok {
			return
		}
		if _, err := s.calls.Accept(id, userID); err != nil {
			return
		}
		s.relayCall(other, env.Type, map[string]any{"callId": id, "sdp": sdp})
	case "call.ice_candidate":
		if e.State != call.Ringing && e.State != call.Accepted {
			return
		}
		var cand map[string]any
		if json.Unmarshal(fields["candidate"], &cand) != nil || len(cand) > 8 {
			return
		}
		s.relayCall(other, env.Type, map[string]any{"callId": id, "candidate": cand})
	case "call.reject", "call.cancel", "call.end", "call.busy":
		if env.Type == "call.reject" && userID != e.CalleeID {
			return
		}
		if env.Type == "call.reject" && e.State != call.Ringing {
			return
		}
		if env.Type == "call.cancel" && userID != e.CallerID {
			return
		}
		if env.Type == "call.cancel" && e.State != call.Created && e.State != call.Ringing {
			return
		}
		if env.Type == "call.busy" && userID != e.CalleeID {
			return
		}
		if env.Type == "call.busy" && e.State != call.Ringing {
			return
		}
		if env.Type == "call.end" && e.State != call.Created && e.State != call.Ringing && e.State != call.Accepted {
			return
		}
		if _, err := s.calls.End(id, userID); err != nil {
			return
		}
		s.relayCall(other, env.Type, map[string]any{"callId": id})
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
