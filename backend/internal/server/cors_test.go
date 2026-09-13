package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"together/backend/internal/config"
)

func corsServer() *http.Server {
	return newTestServerHTTP()
}

// newTestServerHTTP builds a minimal server (only the pinger is needed for these
// CORS checks) with AppOrigin set to testOrigin.
func newTestServerHTTP() *http.Server {
	return New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil,
	)
}

func TestCORSAllowsAppOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", testOrigin)
	rec := httptest.NewRecorder()
	corsServer().Handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != testOrigin {
		t.Fatalf("expected ACAO %q, got %q", testOrigin, got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected ACAC true, got %q", got)
	}
}

func TestCORSRejectsOtherOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	rec := httptest.NewRecorder()
	corsServer().Handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no ACAO for a foreign origin, got %q", got)
	}
}

func TestCORSPreflight(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/posts", nil)
	req.Header.Set("Origin", testOrigin)
	rec := httptest.NewRecorder()
	corsServer().Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for preflight, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatal("expected Access-Control-Allow-Methods on preflight")
	}
}
