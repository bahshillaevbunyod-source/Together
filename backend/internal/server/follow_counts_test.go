package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"together/backend/internal/config"
	"together/backend/internal/session"
)

func countsServer(users *fakeUserRepo, follows *fakeFollowRepo) *http.Server {
	return buildServerWithFollows(
		config.Config{Env: "test", Port: "8080"},
		users,
		&fakeSessionRepo{active: &session.Session{UserID: "me-id", ExpiresAt: time.Now().Add(time.Hour)}},
		follows,
	)
}

func getProfileAuth(srv *http.Server, username string, withCookie bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+username, nil)
	if withCookie {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	}
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func decodeProfile(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	return m
}

func TestPublicProfileCountsAnonymous(t *testing.T) {
	users := &fakeUserRepo{usernameUser: mkUser("target-id", "target_user")}
	follows := &fakeFollowRepo{followers: 5, following: 3}
	rec := getProfileAuth(countsServer(users, follows), "target_user", false)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	m := decodeProfile(t, rec)
	if m["followersCount"].(float64) != 5 || m["followingCount"].(float64) != 3 {
		t.Fatalf("counts not propagated: %v", m)
	}
	if m["isFollowing"].(bool) || m["isSelf"].(bool) {
		t.Fatalf("anonymous must have isFollowing=false, isSelf=false: %v", m)
	}
}

func TestPublicProfileIsFollowingTrue(t *testing.T) {
	users := &fakeUserRepo{byIDUser: mkUser("me-id", "me_user"), usernameUser: mkUser("target-id", "target_user")}
	follows := &fakeFollowRepo{is: true}
	rec := getProfileAuth(countsServer(users, follows), "target_user", true)

	m := decodeProfile(t, rec)
	if !m["isFollowing"].(bool) {
		t.Fatalf("expected isFollowing=true: %v", m)
	}
	if m["isSelf"].(bool) {
		t.Fatalf("expected isSelf=false: %v", m)
	}
}

func TestPublicProfileIsFollowingFalse(t *testing.T) {
	users := &fakeUserRepo{byIDUser: mkUser("me-id", "me_user"), usernameUser: mkUser("target-id", "target_user")}
	follows := &fakeFollowRepo{is: false}
	rec := getProfileAuth(countsServer(users, follows), "target_user", true)

	if decodeProfile(t, rec)["isFollowing"].(bool) {
		t.Fatal("expected isFollowing=false")
	}
}

func TestPublicProfileIsSelf(t *testing.T) {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byIDUser: me, usernameUser: me} // target resolves to self
	rec := getProfileAuth(countsServer(users, &fakeFollowRepo{}), "me_user", true)

	m := decodeProfile(t, rec)
	if !m["isSelf"].(bool) {
		t.Fatalf("expected isSelf=true: %v", m)
	}
}

func TestPublicProfileCountsPropagated(t *testing.T) {
	users := &fakeUserRepo{usernameUser: mkUser("target-id", "target_user")}
	follows := &fakeFollowRepo{followers: 7, following: 9}
	m := decodeProfile(t, getProfileAuth(countsServer(users, follows), "target_user", false))
	if m["followersCount"].(float64) != 7 || m["followingCount"].(float64) != 9 {
		t.Fatalf("counts wrong: %v", m)
	}
}

func TestPublicProfileCountError(t *testing.T) {
	users := &fakeUserRepo{usernameUser: mkUser("target-id", "target_user")}
	follows := &fakeFollowRepo{countErr: errForTest}
	rec := getProfileAuth(countsServer(users, follows), "target_user", false)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
