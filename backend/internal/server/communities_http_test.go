package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"together/backend/internal/config"
)

func TestCommunityRoutesRequireAuthentication(t *testing.T) {
	srv := buildServer(config.Config{Env: "test", Port: "8080"}, &fakeUserRepo{}, &fakeSessionRepo{})
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/groups"},
		{http.MethodPost, "/api/v1/groups"},
		{http.MethodGet, "/api/v1/groups/11111111-1111-1111-1111-111111111111"},
		{http.MethodGet, "/api/v1/groups/11111111-1111-1111-1111-111111111111/members"},
		{http.MethodPost, "/api/v1/groups/11111111-1111-1111-1111-111111111111/members"},
		{http.MethodGet, "/api/v1/channels"},
		{http.MethodPost, "/api/v1/channels"},
		{http.MethodGet, "/api/v1/channels/search"},
		{http.MethodPost, "/api/v1/channels/11111111-1111-1111-1111-111111111111/join"},
		{http.MethodPost, "/api/v1/channels/11111111-1111-1111-1111-111111111111/leave"},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, nil)
			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", rec.Code)
			}
		})
	}
}
