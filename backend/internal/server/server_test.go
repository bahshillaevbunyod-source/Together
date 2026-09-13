package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"together/backend/internal/config"
)

// fakePinger lets us exercise /ready without a real database.
type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func newTestServer(p Pinger) *http.Server {
	return New(config.Config{Env: "test", Port: "8080"}, p, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
}

func decodeStatus(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	return body["status"]
}

func TestHealth(t *testing.T) {
	srv := newTestServer(fakePinger{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := decodeStatus(t, rec); got != "ok" {
		t.Fatalf("expected status \"ok\", got %q", got)
	}
}

func TestReadyWhenDBUp(t *testing.T) {
	srv := newTestServer(fakePinger{err: nil})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := decodeStatus(t, rec); got != "ready" {
		t.Fatalf("expected status \"ready\", got %q", got)
	}
}

func TestReadyWhenDBDown(t *testing.T) {
	srv := newTestServer(fakePinger{err: errors.New("connection refused")})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
	if got := decodeStatus(t, rec); got != "unavailable" {
		t.Fatalf("expected status \"unavailable\", got %q", got)
	}
}
