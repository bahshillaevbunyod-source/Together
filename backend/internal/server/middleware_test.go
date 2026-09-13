package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"together/backend/internal/config"
	"together/backend/internal/session"
)

func getPing(srv *http.Server, cookieValue string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/protected/ping", nil)
	if cookieValue != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookieValue})
	}
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func activeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{active: &session.Session{
		UserID: "u1", ExpiresAt: time.Now().Add(time.Hour),
	}}
}

func TestProtectedValidSession(t *testing.T) {
	users := &fakeUserRepo{byIDUser: userWithPassword(t, "strongpass")}
	srv := buildServer(config.Config{Env: "test", Port: "8080"}, users, activeSessionRepo())

	rec := getPing(srv, "raw-token")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestProtectedMissingCookie(t *testing.T) {
	srv := buildServer(config.Config{Env: "test", Port: "8080"}, &fakeUserRepo{}, &fakeSessionRepo{})
	if rec := getPing(srv, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestProtectedInvalidToken(t *testing.T) {
	srv := buildServer(config.Config{Env: "test", Port: "8080"}, &fakeUserRepo{}, &fakeSessionRepo{active: nil})
	if rec := getPing(srv, "raw-token"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestProtectedExpiredSession(t *testing.T) {
	// Expired sessions are filtered by the query -> repo returns ErrNotFound.
	srv := buildServer(config.Config{Env: "test", Port: "8080"}, &fakeUserRepo{}, &fakeSessionRepo{active: nil})
	if rec := getPing(srv, "raw-token"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestProtectedUserNotFound(t *testing.T) {
	srv := buildServer(config.Config{Env: "test", Port: "8080"}, &fakeUserRepo{byIDUser: nil}, activeSessionRepo())
	if rec := getPing(srv, "raw-token"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestProtectedRepositoryError(t *testing.T) {
	sessions := &fakeSessionRepo{activeErr: context.DeadlineExceeded}
	srv := buildServer(config.Config{Env: "test", Port: "8080"}, &fakeUserRepo{}, sessions)

	rec := getPing(srv, "raw-token")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "deadline") {
		t.Fatalf("internal error leaked details: %s", rec.Body.String())
	}
}

func TestMiddlewarePutsUserInContext(t *testing.T) {
	srv := &Server{
		cfg:      config.Config{Env: "test"},
		users:    &fakeUserRepo{byIDUser: userWithPassword(t, "strongpass")},
		sessions: activeSessionRepo(),
	}

	var seen string
	handler := srv.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		u, ok := CurrentUser(r.Context())
		if !ok {
			t.Fatal("expected authenticated user in context")
		}
		seen = u.Username
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw-token"})
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if seen != "alex_01" {
		t.Fatalf("expected user from context (alex_01), got %q", seen)
	}
}
