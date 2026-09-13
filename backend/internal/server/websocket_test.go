package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"together/backend/internal/config"
	"together/backend/internal/user"
)

// wsServer builds a server whose session auth resolves "me-id" when a cookie is
// present, for exercising the pre-upgrade auth/origin checks of GET /ws.
func wsServer() *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	return New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{},
		&fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{},
		&fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{},
		&fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
}

func wsReq(srv *http.Server, withCookie, withOrigin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	if withCookie {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	}
	if withOrigin {
		req.Header.Set("Origin", testOrigin)
	}
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func TestWebSocketUnauthenticated(t *testing.T) {
	rec := wsReq(wsServer(), false, true)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestWebSocketWrongOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	wsServer().Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestWebSocketMissingOrigin(t *testing.T) {
	rec := wsReq(wsServer(), true, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

// TestWebSocketAuthedNonUpgrade confirms an authenticated, correct-origin
// request that is NOT a real WebSocket handshake is rejected by the upgrader
// (not a 401/403), i.e. auth and origin passed.
func TestWebSocketAuthedNonUpgrade(t *testing.T) {
	rec := wsReq(wsServer(), true, true)
	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Fatalf("auth/origin should have passed, got %d", rec.Code)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 bad handshake, got %d", rec.Code)
	}
}
