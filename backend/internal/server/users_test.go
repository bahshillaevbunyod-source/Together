package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"together/backend/internal/config"
)

func getPublicProfile(srv *http.Server, username string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+username, nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func publicServer(users *fakeUserRepo) *http.Server {
	// No session needed — the endpoint is public.
	return buildServer(config.Config{Env: "test", Port: "8080"}, users, &fakeSessionRepo{})
}

func TestPublicProfileExisting(t *testing.T) {
	users := &fakeUserRepo{usernameUser: userWithPassword(t, "strongpass")}
	rec := getPublicProfile(publicServer(users), "alex_01")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestPublicProfileNormalizesUsername(t *testing.T) {
	users := &fakeUserRepo{usernameUser: userWithPassword(t, "strongpass")}
	rec := getPublicProfile(publicServer(users), "%20ALEX_01%20") // " ALEX_01 " url-encoded
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if users.lastUsername != "alex_01" {
		t.Fatalf("username not normalized to lowercase/trimmed, got %q", users.lastUsername)
	}
}

func TestPublicProfileUnknown(t *testing.T) {
	users := &fakeUserRepo{usernameUser: nil} // -> ErrNotFound
	rec := getPublicProfile(publicServer(users), "ghost")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestPublicProfileRepositoryError(t *testing.T) {
	users := &fakeUserRepo{usernameErr: errForTest}
	rec := getPublicProfile(publicServer(users), "alex_01")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestPublicProfileHidesPrivateFields(t *testing.T) {
	users := &fakeUserRepo{usernameUser: userWithPassword(t, "strongpass")}
	rec := getPublicProfile(publicServer(users), "alex_01")
	body := rec.Body.String()
	for _, forbidden := range []string{"email", "phone", "password"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("public profile leaked %q: %s", forbidden, body)
		}
	}
}
