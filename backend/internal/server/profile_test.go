package server

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"together/backend/internal/config"
	"together/backend/internal/session"
)

var errForTest = errors.New("boom")

func profileServer(users *fakeUserRepo) *http.Server {
	return buildServer(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		users,
		&fakeSessionRepo{active: &session.Session{UserID: "u1", ExpiresAt: time.Now().Add(time.Hour)}},
	)
}

func profileUser(t *testing.T) *fakeUserRepo {
	t.Helper()
	u := userWithPassword(t, "strongpass")
	return &fakeUserRepo{byIDUser: u, updateUser: u}
}

func getProfile(srv *http.Server, withCookie bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profile", nil)
	if withCookie {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	}
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func patchProfile(srv *http.Server, body string, withCookie, withOrigin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/profile", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if withCookie {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	}
	if withOrigin {
		req.Header.Set("Origin", testOrigin)
	}
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func TestGetProfile(t *testing.T) {
	rec := getProfile(profileServer(profileUser(t)), true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Fatalf("profile leaked password data: %s", rec.Body.String())
	}
}

func TestPatchProfileOneField(t *testing.T) {
	rec := patchProfile(profileServer(profileUser(t)), `{"displayName":"New Name"}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestPatchProfileSeveralFields(t *testing.T) {
	body := `{"displayName":"New Name","bio":"hello","countryCode":"US","city":"Tashkent","nativeLanguage":"uz","avatarUrl":"https://x/y.png"}`
	rec := patchProfile(profileServer(profileUser(t)), body, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestPatchProfileEmpty(t *testing.T) {
	rec := patchProfile(profileServer(profileUser(t)), `{}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestPatchProfileInvalidCountryCode(t *testing.T) {
	rec := patchProfile(profileServer(profileUser(t)), `{"countryCode":"usa"}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestPatchProfileBioTooLong(t *testing.T) {
	long := strings.Repeat("a", 301)
	rec := patchProfile(profileServer(profileUser(t)), `{"bio":"`+long+`"}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestProfileUnauthenticated(t *testing.T) {
	users := &fakeUserRepo{} // GetByID -> ErrNotFound, but no cookie anyway
	if rec := getProfile(profileServer(users), false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET expected 401, got %d", rec.Code)
	}
	if rec := patchProfile(profileServer(users), `{"displayName":"X"}`, false, true); rec.Code != http.StatusUnauthorized {
		t.Fatalf("PATCH expected 401, got %d", rec.Code)
	}
}

func TestPatchProfileMissingOrigin(t *testing.T) {
	rec := patchProfile(profileServer(profileUser(t)), `{"displayName":"New Name"}`, true, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestPatchProfileRepositoryError(t *testing.T) {
	users := profileUser(t)
	users.updateErr = errForTest
	users.updateUser = nil
	rec := patchProfile(profileServer(users), `{"displayName":"New Name"}`, true, true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
