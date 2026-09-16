package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"together/backend/internal/config"
	"together/backend/internal/session"
	"together/backend/internal/user"
)

func getPublicProfile(srv *http.Server, username string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+username, nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func getPublicProfileAs(srv *http.Server, username string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+username, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
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

func profileAccessServer(blocks *fakeBlockRepo, viewer, target *user.User) *http.Server {
	return buildServerFull(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		&fakeUserRepo{byIDUser: viewer, usernameUser: target},
		&fakeSessionRepo{active: &session.Session{UserID: viewer.ID, ExpiresAt: time.Now().Add(time.Hour)}},
		&fakeFollowRepo{}, blocks,
	)
}

func TestPublicProfileBlockedBothDirections(t *testing.T) {
	for _, tc := range []struct {
		name   string
		viewer string
		target string
	}{
		{name: "blocker cannot view blocked", viewer: "a", target: "b"},
		{name: "blocked cannot view blocker", viewer: "b", target: "a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			viewer := mkUser(tc.viewer, tc.viewer+"_user")
			target := mkUser(tc.target, tc.target+"_user")
			srv := profileAccessServer(&fakeBlockRepo{hasBetween: true}, viewer, target)
			rec := getPublicProfileAs(srv, target.Username)
			if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "user not found") {
				t.Fatalf("expected hidden profile 404, got %d (%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestPublicProfileBlockCheckError(t *testing.T) {
	viewer := mkUser("viewer", "viewer_user")
	target := mkUser("target", "target_user")
	rec := getPublicProfileAs(profileAccessServer(&fakeBlockRepo{betweenErr: errForTest}, viewer, target), target.Username)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestPublicProfileUnrelatedAndSelfRemainAccessible(t *testing.T) {
	viewer := mkUser("viewer", "viewer_user")
	target := mkUser("target", "target_user")
	if rec := getPublicProfileAs(profileAccessServer(&fakeBlockRepo{}, viewer, target), target.Username); rec.Code != http.StatusOK {
		t.Fatalf("unrelated profile expected 200, got %d", rec.Code)
	}
	self := mkUser("self", "self_user")
	if rec := getPublicProfileAs(profileAccessServer(&fakeBlockRepo{hasBetween: true}, self, self), self.Username); rec.Code != http.StatusOK {
		t.Fatalf("self profile expected 200, got %d", rec.Code)
	}
}
