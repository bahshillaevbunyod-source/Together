package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"together/backend/internal/config"
	"together/backend/internal/session"
)

func followBlockSetup(t *testing.T, blocks *fakeBlockRepo) *http.Server {
	t.Helper()
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byIDUser: me, usernameUser: mkUser("target-id", "target_user")}
	return buildServerFull(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		users,
		&fakeSessionRepo{active: &session.Session{UserID: "me-id", ExpiresAt: time.Now().Add(time.Hour)}},
		&fakeFollowRepo{},
		blocks,
	)
}

func followBlockReq(srv *http.Server, method string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/v1/users/target_user/follow", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	req.Header.Set("Origin", testOrigin)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func TestFollowAllowedWithoutBlock(t *testing.T) {
	rec := followBlockReq(followBlockSetup(t, &fakeBlockRepo{hasBetween: false}), http.MethodPost)
	if rec.Code != http.StatusOK {
		t.Fatalf("normal follow should be 200, got %d", rec.Code)
	}
}

func TestFollowBlockedForbidden(t *testing.T) {
	// hasBetween=true models a block in either direction (direction hidden).
	rec := followBlockReq(followBlockSetup(t, &fakeBlockRepo{hasBetween: true}), http.MethodPost)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("blocked follow should be 403, got %d", rec.Code)
	}
}

func TestFollowBlockedResponseHidesDirection(t *testing.T) {
	rec := followBlockReq(followBlockSetup(t, &fakeBlockRepo{hasBetween: true}), http.MethodPost)
	body := strings.TrimSpace(rec.Body.String())
	if body != `{"error":"interaction not allowed"}` {
		t.Fatalf("unexpected body: %s", body)
	}
	for _, leak := range []string{"blocker", "blocked", "me-id", "target-id"} {
		if strings.Contains(body, leak) {
			t.Fatalf("response leaked block direction (%q): %s", leak, body)
		}
	}
}

func TestFollowBlockCheckError(t *testing.T) {
	rec := followBlockReq(followBlockSetup(t, &fakeBlockRepo{betweenErr: errForTest}), http.MethodPost)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("block-check error should be 500, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "boom") {
		t.Fatalf("internal error leaked details: %s", rec.Body.String())
	}
}

func TestUnfollowAllowedDespiteBlock(t *testing.T) {
	// Unblock/unfollow must stay idempotent and allowed even with a block.
	rec := followBlockReq(followBlockSetup(t, &fakeBlockRepo{hasBetween: true}), http.MethodDelete)
	if rec.Code != http.StatusOK {
		t.Fatalf("unfollow with block should still be 200, got %d", rec.Code)
	}
}
