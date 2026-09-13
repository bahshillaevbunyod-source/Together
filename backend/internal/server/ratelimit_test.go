package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"together/backend/internal/config"
)

const (
	registerPath = "/api/v1/auth/register"
	loginPath    = "/api/v1/auth/login"
)

func rlConfig() config.Config {
	return config.Config{Env: "test", Port: "8080"}
}

func postWithIP(srv *http.Server, path, body, ip string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = ip + ":40000"
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func TestRegisterWithinLimit(t *testing.T) {
	srv := buildServer(rlConfig(), &fakeUserRepo{}, &fakeSessionRepo{})
	for i := 0; i < 5; i++ {
		if rec := postWithIP(srv, registerPath, validBody, "10.0.0.1"); rec.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d within limit was rate limited", i+1)
		}
	}
}

func TestRegisterExceedsLimit(t *testing.T) {
	srv := buildServer(rlConfig(), &fakeUserRepo{}, &fakeSessionRepo{})
	for i := 0; i < 5; i++ {
		postWithIP(srv, registerPath, validBody, "10.0.0.2")
	}
	rec := postWithIP(srv, registerPath, validBody, "10.0.0.2")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after limit, got %d", rec.Code)
	}
	ra := rec.Header().Get("Retry-After")
	if ra == "" {
		t.Fatal("Retry-After header missing")
	}
	if secs, err := strconv.Atoi(ra); err != nil || secs <= 0 {
		t.Fatalf("Retry-After must be a positive integer, got %q", ra)
	}
}

func TestLoginWithinLimit(t *testing.T) {
	users := &fakeUserRepo{getUser: userWithPassword(t, "strongpass")}
	srv := buildServer(rlConfig(), users, &fakeSessionRepo{})
	for i := 0; i < 10; i++ {
		if rec := postWithIP(srv, loginPath, loginBody, "10.0.0.3"); rec.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d within limit was rate limited", i+1)
		}
	}
}

func TestLoginExceedsLimit(t *testing.T) {
	users := &fakeUserRepo{getUser: userWithPassword(t, "strongpass")}
	srv := buildServer(rlConfig(), users, &fakeSessionRepo{})
	for i := 0; i < 10; i++ {
		postWithIP(srv, loginPath, loginBody, "10.0.0.4")
	}
	if rec := postWithIP(srv, loginPath, loginBody, "10.0.0.4"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after limit, got %d", rec.Code)
	}
}

func TestRateLimitIndependentPerIP(t *testing.T) {
	srv := buildServer(rlConfig(), &fakeUserRepo{}, &fakeSessionRepo{})

	// Exhaust IP A (6th exceeds the limit of 5).
	for i := 0; i < 6; i++ {
		postWithIP(srv, registerPath, validBody, "10.0.0.20")
	}
	// A different IP must still be allowed on its first request.
	if rec := postWithIP(srv, registerPath, validBody, "10.0.0.21"); rec.Code == http.StatusTooManyRequests {
		t.Fatalf("different IP should have an independent bucket, got 429")
	}
}
