package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"together/backend/internal/config"
)

const testOrigin = "http://localhost:3000"

func csrfServer() *Server {
	return &Server{cfg: config.Config{Env: "test", AppOrigin: testOrigin}}
}

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func runCSRF(method, origin string) *httptest.ResponseRecorder {
	handler := csrfServer().csrfProtect(okHandler)
	req := httptest.NewRequest(method, "/x", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func TestCSRFSafeGetNoOriginPasses(t *testing.T) {
	if rec := runCSRF(http.MethodGet, ""); rec.Code != http.StatusOK {
		t.Fatalf("safe GET without Origin should pass, got %d", rec.Code)
	}
}

func TestCSRFStateChangingCorrectOrigin(t *testing.T) {
	if rec := runCSRF(http.MethodPost, testOrigin); rec.Code != http.StatusOK {
		t.Fatalf("POST with correct Origin should pass, got %d", rec.Code)
	}
}

func TestCSRFStateChangingMissingOrigin(t *testing.T) {
	if rec := runCSRF(http.MethodPost, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("POST without Origin should be 403, got %d", rec.Code)
	}
}

func TestCSRFStateChangingWrongOrigin(t *testing.T) {
	if rec := runCSRF(http.MethodPost, "https://evil.example.com"); rec.Code != http.StatusForbidden {
		t.Fatalf("POST with wrong Origin should be 403, got %d", rec.Code)
	}
}

// A protected GET behind requireAuth(csrfProtect(...)) must work without Origin.
func TestAuthenticatedGetNoOriginPasses(t *testing.T) {
	srv := &Server{
		cfg:      config.Config{Env: "test", AppOrigin: testOrigin},
		users:    &fakeUserRepo{byIDUser: userWithPassword(t, "strongpass")},
		sessions: activeSessionRepo(),
	}
	handler := srv.requireAuth(srv.csrfProtect(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw-token"})
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated GET without Origin should pass, got %d", rec.Code)
	}
}

func TestLogoutCorrectOrigin(t *testing.T) {
	srv := buildServer(config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin}, &fakeUserRepo{}, &fakeSessionRepo{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Origin", testOrigin)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout with correct Origin should be 200, got %d", rec.Code)
	}
}

func TestLogoutMissingOrigin(t *testing.T) {
	srv := buildServer(config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin}, &fakeUserRepo{}, &fakeSessionRepo{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("logout without Origin should be 403, got %d", rec.Code)
	}
}
