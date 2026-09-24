package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"together/backend/internal/config"
	"together/backend/internal/conversation"
	"together/backend/internal/session"
	"together/backend/internal/user"
)

// End-to-end call signaling over REAL WebSocket connections: a real HTTP
// server, cookie-authenticated sockets for several users, the real hub, read
// and write pumps, registry and REST call creation. Only persistence is faked.

const (
	convAB = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	convCB = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
)

type multiSessionRepo struct {
	mu     sync.Mutex
	byHash map[string]string // token hash -> user id
}

func (m *multiSessionRepo) Create(context.Context, string, string, time.Time) (*session.Session, error) {
	return nil, session.ErrNotFound
}
func (m *multiSessionRepo) GetActiveByTokenHash(_ context.Context, hash string) (*session.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byHash[hash]
	if !ok {
		return nil, session.ErrNotFound
	}
	return &session.Session{UserID: id, ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (m *multiSessionRepo) DeleteByTokenHash(context.Context, string) error { return nil }

// directConvRepo adds the optional DirectPeer lookup to the conversation fake.
type directConvRepo struct {
	*fakeConversationRepo
	pairs map[string][2]string // conversation id -> participants
}

func (d *directConvRepo) DirectPeer(_ context.Context, conversationID, userID string) (string, string, error) {
	p, ok := d.pairs[conversationID]
	if !ok || (userID != p[0] && userID != p[1]) {
		return "", "", conversation.ErrNotParticipant
	}
	if userID == p[0] {
		return "direct", p[1], nil
	}
	return "direct", p[0], nil
}

type callHarness struct {
	t      *testing.T
	srv    *httptest.Server
	s      *Server
	blocks *fakeBlockRepo
}

func newCallHarness(t *testing.T) *callHarness {
	t.Helper()
	users := &fakeUserRepo{byID: map[string]*user.User{
		"user-a": mkUser("user-a", "alice"),
		"user-b": mkUser("user-b", "bob"),
		"user-c": mkUser("user-c", "carol"),
	}}
	sessions := &multiSessionRepo{byHash: map[string]string{
		session.HashToken("tok-a"): "user-a",
		session.HashToken("tok-b"): "user-b",
		session.HashToken("tok-c"): "user-c",
	}}
	blocks := &fakeBlockRepo{}
	convs := &directConvRepo{fakeConversationRepo: &fakeConversationRepo{}, pairs: map[string][2]string{
		convAB: {"user-a", "user-b"},
		convCB: {"user-c", "user-b"},
	}}
	s := newServer(config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users, sessions, &fakeFollowRepo{}, blocks,
		&fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{},
		&fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{},
		&fakeLikeNotifier{}, &fakeCommentNotifier{}, convs)
	srv := httptest.NewServer(s.routes())
	t.Cleanup(srv.Close)
	return &callHarness{t: t, srv: srv, s: s, blocks: blocks}
}

// wsClient reads frames on a background goroutine so tests can assert both
// delivery and silence without a read deadline (a timed-out gorilla read
// leaves the connection unusable).
type wsClient struct {
	conn   *websocket.Conn
	frames chan frame
}

func (c *wsClient) Close() error { return c.conn.Close() }

func (h *callHarness) dial(token string) *wsClient {
	h.t.Helper()
	hdr := http.Header{}
	hdr.Set("Origin", testOrigin)
	hdr.Set("Cookie", sessionCookieName+"="+token)
	url := "ws" + strings.TrimPrefix(h.srv.URL, "http") + "/api/v1/ws"
	conn, resp, err := websocket.DefaultDialer.Dial(url, hdr)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		h.t.Fatalf("dial %s: %v (status %d)", token, err, status)
	}
	h.t.Cleanup(func() { _ = conn.Close() })
	c := &wsClient{conn: conn, frames: make(chan frame, 32)}
	go func() {
		defer close(c.frames)
		for {
			var f frame
			if err := conn.ReadJSON(&f); err != nil {
				return
			}
			c.frames <- f
		}
	}()
	// Registration happens in the handler before the read pump; wait for it.
	h.waitConnections(1)
	return c
}

func (h *callHarness) waitConnections(min int) {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if h.s.hub.TotalConnections() >= min {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (h *callHarness) createCall(token, conv, typ string) (int, string) {
	h.t.Helper()
	body, _ := json.Marshal(map[string]string{"conversationId": conv, "type": typ})
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/api/v1/calls", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", testOrigin)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		CallID string `json:"callId"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out.CallID
}

type frame struct {
	Type string                     `json:"type"`
	Data map[string]json.RawMessage `json:"data"`
}

func send(t *testing.T, c *wsClient, typ string, data map[string]any) {
	t.Helper()
	if err := c.conn.WriteJSON(map[string]any{"type": typ, "data": data}); err != nil {
		t.Fatalf("write %s: %v", typ, err)
	}
}

func expectFrame(t *testing.T, c *wsClient, typ string) frame {
	t.Helper()
	select {
	case f, ok := <-c.frames:
		if !ok {
			t.Fatalf("expected %s, connection closed", typ)
		}
		if f.Type != typ {
			t.Fatalf("expected %s, got %s", typ, f.Type)
		}
		return f
	case <-time.After(2 * time.Second):
		t.Fatalf("expected %s, got nothing", typ)
	}
	return frame{}
}

func expectSilence(t *testing.T, c *wsClient) {
	t.Helper()
	select {
	case f, ok := <-c.frames:
		if ok {
			t.Fatalf("unexpected frame %s", f.Type)
		}
	case <-time.After(150 * time.Millisecond):
	}
}

func str(raw json.RawMessage) string {
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

func TestCallSignalingEndToEndOverWebSockets(t *testing.T) {
	h := newCallHarness(t)
	a := h.dial("tok-a")
	b := h.dial("tok-b")
	c := h.dial("tok-c")
	h.waitConnections(3)

	code, id := h.createCall("tok-a", convAB, "video")
	if code != http.StatusCreated || id == "" {
		t.Fatalf("create call: %d %q", code, id)
	}

	// Stage 1: A's offer reaches B (and only B) with server-built identity.
	send(t, a, "call.offer", map[string]any{"callId": id, "sdp": "offer-sdp"})
	offer := expectFrame(t, b, "call.offer")
	if str(offer.Data["callId"]) != id || str(offer.Data["callType"]) != "video" || str(offer.Data["conversationId"]) != convAB || str(offer.Data["sdp"]) != "offer-sdp" {
		t.Fatalf("bad offer payload: %v", offer.Data)
	}
	var caller map[string]any
	_ = json.Unmarshal(offer.Data["caller"], &caller)
	if caller["id"] != "user-a" || caller["username"] != "alice" {
		t.Fatalf("caller identity not server-built: %v", caller)
	}
	expectSilence(t, c)

	// Early ICE from A while ringing is relayed (the callee queues it).
	send(t, a, "call.ice_candidate", map[string]any{"callId": id, "candidate": map[string]any{"candidate": "c1", "sdpMid": "0", "sdpMLineIndex": 0}})
	expectFrame(t, b, "call.ice_candidate")

	// Unrelated C cannot answer, end or inject ICE into the call.
	send(t, c, "call.answer", map[string]any{"callId": id, "sdp": "evil"})
	send(t, c, "call.end", map[string]any{"callId": id})
	send(t, c, "call.ice_candidate", map[string]any{"callId": id, "candidate": map[string]any{"candidate": "x"}})
	expectSilence(t, a)
	expectSilence(t, b)

	// Stage 2: B's answer returns to A.
	send(t, b, "call.answer", map[string]any{"callId": id, "sdp": "answer-sdp"})
	ans := expectFrame(t, a, "call.answer")
	if str(ans.Data["sdp"]) != "answer-sdp" {
		t.Fatalf("bad answer: %v", ans.Data)
	}

	// Stage 3: ICE both directions.
	send(t, b, "call.ice_candidate", map[string]any{"callId": id, "candidate": map[string]any{"candidate": "c2", "sdpMid": "0", "sdpMLineIndex": 0}})
	expectFrame(t, a, "call.ice_candidate")
	send(t, a, "call.ice_candidate", map[string]any{"callId": id, "candidate": map[string]any{"candidate": "c3", "sdpMid": "0", "sdpMLineIndex": 0}})
	expectFrame(t, b, "call.ice_candidate")

	// Busy: C cannot call B while A↔B is active.
	if code, _ := h.createCall("tok-c", convCB, "voice"); code != http.StatusConflict {
		t.Fatalf("expected 409 busy, got %d", code)
	}

	// End by B → A notified, registry cleaned, a new call starts at once.
	send(t, b, "call.end", map[string]any{"callId": id})
	expectFrame(t, a, "call.end")
	if _, ok := h.s.calls.Get(id); ok {
		t.Fatal("ended call still registered")
	}
	if code, _ := h.createCall("tok-a", convAB, "voice"); code != http.StatusCreated {
		t.Fatalf("new call after end should succeed, got %d", code)
	}
}

func TestCallRejectCancelAndBusySignals(t *testing.T) {
	h := newCallHarness(t)
	a := h.dial("tok-a")
	b := h.dial("tok-b")
	h.waitConnections(2)

	// Reject: only the callee may reject a ringing call.
	_, id := h.createCall("tok-a", convAB, "voice")
	send(t, a, "call.offer", map[string]any{"callId": id, "sdp": "o"})
	expectFrame(t, b, "call.offer")
	send(t, a, "call.reject", map[string]any{"callId": id, "reason": "declined"}) // caller cannot reject
	expectSilence(t, b)
	send(t, b, "call.reject", map[string]any{"callId": id, "reason": "declined"})
	expectFrame(t, a, "call.reject")
	if _, ok := h.s.calls.Get(id); ok {
		t.Fatal("rejected call still registered")
	}

	// Cancel while ringing: callee is told, reservations released.
	_, id = h.createCall("tok-a", convAB, "video")
	send(t, a, "call.offer", map[string]any{"callId": id, "sdp": "o"})
	expectFrame(t, b, "call.offer")
	send(t, a, "call.cancel", map[string]any{"callId": id, "reason": "cancelled"})
	expectFrame(t, b, "call.cancel")

	// Busy signal from the callee ends the call for the caller.
	_, id = h.createCall("tok-a", convAB, "voice")
	send(t, a, "call.offer", map[string]any{"callId": id, "sdp": "o"})
	expectFrame(t, b, "call.offer")
	send(t, b, "call.busy", map[string]any{"callId": id})
	expectFrame(t, a, "call.busy")
	if code, _ := h.createCall("tok-a", convAB, "voice"); code != http.StatusCreated {
		t.Fatalf("reservations must be released after busy, got %d", code)
	}
}

func TestCallDisconnectEndsCallForPeer(t *testing.T) {
	h := newCallHarness(t)
	a := h.dial("tok-a")
	b := h.dial("tok-b")
	h.waitConnections(2)
	_, id := h.createCall("tok-a", convAB, "voice")
	send(t, a, "call.offer", map[string]any{"callId": id, "sdp": "o"})
	expectFrame(t, b, "call.offer")
	send(t, b, "call.answer", map[string]any{"callId": id, "sdp": "a"})
	expectFrame(t, a, "call.answer")

	_ = a.Close() // caller's tab closes
	expectFrame(t, b, "call.end")
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, ok := h.s.calls.Get(id); !ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("disconnect left the call registered")
}

func TestCallOfferToOfflineCalleeIsRejectedAndReleased(t *testing.T) {
	h := newCallHarness(t)
	a := h.dial("tok-a") // B never connects
	_, id := h.createCall("tok-a", convAB, "voice")
	send(t, a, "call.offer", map[string]any{"callId": id, "sdp": "o"})
	expectFrame(t, a, "call.reject")
	if code, _ := h.createCall("tok-a", convAB, "voice"); code != http.StatusCreated {
		t.Fatalf("offline callee must not leave a reservation, got %d", code)
	}
}

func TestCallCreationSecurity(t *testing.T) {
	h := newCallHarness(t)
	// Not a participant of A↔B.
	if code, _ := h.createCall("tok-c", convAB, "voice"); code != http.StatusNotFound {
		t.Fatalf("non-participant: expected 404, got %d", code)
	}
	// Invalid call type.
	if code, _ := h.createCall("tok-a", convAB, "screen"); code != http.StatusBadRequest {
		t.Fatalf("invalid type: expected 400, got %d", code)
	}
	// Blocked in either direction.
	h.blocks.hasBetween = true
	if code, _ := h.createCall("tok-a", convAB, "voice"); code != http.StatusForbidden {
		t.Fatalf("blocked: expected 403, got %d", code)
	}
}

func TestCallOversizedFrameClosesSocket(t *testing.T) {
	h := newCallHarness(t)
	a := h.dial("tok-a")
	big := strings.Repeat("x", wsMaxMessageBytes+1)
	_ = a.conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"call.offer","data":{"callId":"x","sdp":"`+big+`"}}`))
	select {
	case _, ok := <-a.frames:
		if ok {
			t.Fatal("oversized frame should close the connection, got a frame")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("oversized frame should close the connection")
	}
}
