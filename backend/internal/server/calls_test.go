package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"together/backend/internal/call"
	"together/backend/internal/config"
	"together/backend/internal/realtime"
	"together/backend/internal/user"
)

func TestCallsRequireAuthentication(t *testing.T) {
	s := newTestServer(fakePinger{})
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/calls/config"},
		{http.MethodPost, "/api/v1/calls"},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			s.Handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", rec.Code)
			}
		})
	}
}

func TestCallOfferRelaysOnlyToCalleeWithServerIdentity(t *testing.T) {
	a := &user.User{ID: "caller", Username: "caller_name", DisplayName: "Caller"}
	b := &user.User{ID: "callee", Username: "callee_name", DisplayName: "Callee"}
	s := newServer(config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin}, fakePinger{}, &fakeUserRepo{byID: map[string]*user.User{
		a.ID: a,
		b.ID: b,
	}}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	aConn := realtime.NewClient(a.ID, 4)
	bConn := realtime.NewClient(b.ID, 4)
	cConn := realtime.NewClient("unrelated", 4)
	s.hub.Register(aConn)
	s.hub.Register(bConn)
	s.hub.Register(cConn)

	e, err := s.calls.Create("11111111-1111-1111-1111-111111111111", a.ID, b.ID, call.Voice)
	if err != nil {
		t.Fatal(err)
	}
	s.handleCallFrame(a.ID, []byte(`{"type":"call.offer","data":{"callId":"`+e.ID+`","sdp":"v=0"}}`))

	select {
	case raw := <-bConn.Send():
		var frame struct {
			Type string `json:"type"`
			Data struct {
				CallID         string `json:"callId"`
				ConversationID string `json:"conversationId"`
				CallType       string `json:"callType"`
				Caller         struct {
					ID          string `json:"id"`
					Username    string `json:"username"`
					DisplayName string `json:"displayName"`
				} `json:"caller"`
				SDP string `json:"sdp"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &frame); err != nil {
			t.Fatal(err)
		}
		if frame.Type != "call.offer" || frame.Data.CallID != e.ID || frame.Data.ConversationID != e.ConversationID || frame.Data.CallType != "voice" || frame.Data.SDP != "v=0" {
			t.Fatalf("unexpected callee frame: %s", raw)
		}
		if frame.Data.Caller.ID != a.ID || frame.Data.Caller.Username != a.Username || frame.Data.Caller.DisplayName != a.DisplayName {
			t.Fatalf("caller identity was not server-built: %s", raw)
		}
	case <-time.After(time.Second):
		t.Fatal("callee did not receive call offer")
	}
	if _, ok := s.calls.Get(e.ID); !ok {
		t.Fatal("call disappeared after offer")
	}
	if state, _ := s.calls.Get(e.ID); state.State != call.Ringing {
		t.Fatalf("expected ringing state, got %s", state.State)
	}
	select {
	case <-aConn.Send():
		t.Fatal("caller received its own offer")
	default:
	}
	select {
	case <-cConn.Send():
		t.Fatal("unrelated user received call offer")
	default:
	}
}
