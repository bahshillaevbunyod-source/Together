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
	"together/backend/internal/storage"
	"together/backend/internal/user"
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
	body := `{"displayName":"New Name","bio":"hello","countryCode":"US","city":"Tashkent","nativeLanguage":"uz"}`
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

// --- Avatar lifecycle cleanup -----------------------------------------------

const avatarBase = "https://cdn.example.com/media"

// avatarCleanupServer builds a profile server with a storage double and a real
// media public base so avatar URLs round-trip to storage keys.
func avatarCleanupServer(users *fakeUserRepo, sr *fakeStorageRepo) *http.Server {
	if sr.headInfo == nil && sr.headErr == nil {
		sr.headInfo = &storage.ObjectInfo{ContentType: "image/png", SizeBytes: 1000}
	}
	return buildServerWithStorage(
		config.Config{
			Env:                "test",
			Port:               "8080",
			AppOrigin:          testOrigin,
			MediaPublicBaseURL: avatarBase,
		},
		users,
		&fakeSessionRepo{active: &session.Session{UserID: "u1", ExpiresAt: time.Now().Add(time.Hour)}},
		sr,
	)
}

// userWithAvatar clones a valid stored user and sets its avatar URL.
func userWithAvatar(t *testing.T, url *string) *user.User {
	t.Helper()
	u := userWithPassword(t, "strongpass")
	u.AvatarURL = url
	return u
}

func strptr(s string) *string { return &s }

func TestAvatarValidOwnedObjectAcceptedAndCanonicalized(t *testing.T) {
	key := "users/11111111-1111-1111-1111-111111111111/avatars/new.png"
	submitted := avatarBase + "/" + key
	users := profileUser(t)
	sr := &fakeStorageRepo{headInfo: &storage.ObjectInfo{ContentType: "image/png", SizeBytes: 1000}}

	rec := patchProfile(avatarCleanupServer(users, sr), `{"avatarUrl":"`+submitted+`"}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !users.lastUpdate.AvatarURL.Set || users.lastUpdate.AvatarURL.Value == nil {
		t.Fatal("expected avatar update")
	}
	want := mediaURL(avatarBase, key)
	if *users.lastUpdate.AvatarURL.Value != want {
		t.Fatalf("expected canonical avatar URL %q, got %q", want, *users.lastUpdate.AvatarURL.Value)
	}
}

func TestAvatarExternalURLRejected(t *testing.T) {
	users := profileUser(t)
	rec := patchProfile(avatarCleanupServer(users, &fakeStorageRepo{}), `{"avatarUrl":"https://external.example/avatar.png"}`, true, true)
	if rec.Code != http.StatusBadRequest || users.lastUpdate.HasChanges() {
		t.Fatalf("expected rejected external avatar without persistence, got %d", rec.Code)
	}
}

func TestAvatarForeignUserRejected(t *testing.T) {
	users := profileUser(t)
	url := avatarBase + "/users/22222222-2222-2222-2222-222222222222/avatars/a.png"
	rec := patchProfile(avatarCleanupServer(users, &fakeStorageRepo{}), `{"avatarUrl":"`+url+`"}`, true, true)
	if rec.Code != http.StatusBadRequest || users.lastUpdate.HasChanges() {
		t.Fatalf("expected rejected foreign avatar without persistence, got %d", rec.Code)
	}
}

func TestAvatarPostMediaPathRejected(t *testing.T) {
	users := profileUser(t)
	url := avatarBase + "/users/11111111-1111-1111-1111-111111111111/uploads/post.png"
	rec := patchProfile(avatarCleanupServer(users, &fakeStorageRepo{}), `{"avatarUrl":"`+url+`"}`, true, true)
	if rec.Code != http.StatusBadRequest || users.lastUpdate.HasChanges() {
		t.Fatalf("expected rejected post-media avatar without persistence, got %d", rec.Code)
	}
}

func TestAvatarMissingObjectRejected(t *testing.T) {
	users := profileUser(t)
	sr := &fakeStorageRepo{headErr: storage.ErrObjectNotFound}
	url := avatarBase + "/users/11111111-1111-1111-1111-111111111111/avatars/missing.png"
	rec := patchProfile(avatarCleanupServer(users, sr), `{"avatarUrl":"`+url+`"}`, true, true)
	if rec.Code != http.StatusBadRequest || users.lastUpdate.HasChanges() {
		t.Fatalf("expected rejected missing avatar without persistence, got %d", rec.Code)
	}
}

func TestAvatarInvalidObjectRejected(t *testing.T) {
	users := profileUser(t)
	sr := &fakeStorageRepo{headInfo: &storage.ObjectInfo{ContentType: "image/gif", SizeBytes: 1000}}
	url := avatarBase + "/users/11111111-1111-1111-1111-111111111111/avatars/invalid.gif"
	rec := patchProfile(avatarCleanupServer(users, sr), `{"avatarUrl":"`+url+`"}`, true, true)
	if rec.Code != http.StatusBadRequest || users.lastUpdate.HasChanges() {
		t.Fatalf("expected rejected invalid avatar without persistence, got %d", rec.Code)
	}
}

func TestAvatarNonImageRejected(t *testing.T) {
	users := profileUser(t)
	sr := &fakeStorageRepo{headInfo: &storage.ObjectInfo{ContentType: "video/mp4", SizeBytes: 1000}}
	url := avatarBase + "/users/11111111-1111-1111-1111-111111111111/avatars/video.mp4"
	rec := patchProfile(avatarCleanupServer(users, sr), `{"avatarUrl":"`+url+`"}`, true, true)
	if rec.Code != http.StatusBadRequest || users.lastUpdate.HasChanges() {
		t.Fatalf("expected rejected non-image avatar without persistence, got %d", rec.Code)
	}
}

func TestAvatarNullRemovesAvatar(t *testing.T) {
	users := profileUser(t)
	rec := patchProfile(avatarCleanupServer(users, &fakeStorageRepo{}), `{"avatarUrl":null}`, true, true)
	if rec.Code != http.StatusOK || !users.lastUpdate.AvatarURL.Set || users.lastUpdate.AvatarURL.Value != nil {
		t.Fatalf("expected null avatar removal, got %d", rec.Code)
	}
}

func TestAvatarOmittedPreservesLegacyAvatar(t *testing.T) {
	legacy := avatarBase + "/users/11111111-1111-1111-1111-111111111111/uploads/legacy.png"
	u := userWithAvatar(t, strptr(legacy))
	users := &fakeUserRepo{byIDUser: u, updateUser: u}
	rec := patchProfile(avatarCleanupServer(users, &fakeStorageRepo{}), `{"displayName":"New Name"}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if users.lastUpdate.AvatarURL.Set {
		t.Fatal("omitted avatarUrl must remain unset")
	}
}

func TestAvatarReplaceDeletesPrevious(t *testing.T) {
	oldURL := avatarBase + "/users/11111111-1111-1111-1111-111111111111/avatars/old.png"
	newURL := avatarBase + "/users/11111111-1111-1111-1111-111111111111/avatars/new.png"
	users := &fakeUserRepo{
		byIDUser:   userWithAvatar(t, strptr(oldURL)),
		updateUser: userWithAvatar(t, strptr(newURL)),
	}
	sr := &fakeStorageRepo{}
	rec := patchProfile(avatarCleanupServer(users, sr), `{"avatarUrl":"`+newURL+`"}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	want := "users/11111111-1111-1111-1111-111111111111/avatars/old.png"
	if len(sr.deleted) != 1 || sr.deleted[0] != want {
		t.Fatalf("expected previous avatar deleted (%s), got %v", want, sr.deleted)
	}
}

func TestAvatarRemoveDeletesPrevious(t *testing.T) {
	oldURL := avatarBase + "/users/11111111-1111-1111-1111-111111111111/avatars/old.png"
	users := &fakeUserRepo{
		byIDUser:   userWithAvatar(t, strptr(oldURL)),
		updateUser: userWithAvatar(t, nil),
	}
	sr := &fakeStorageRepo{}
	rec := patchProfile(avatarCleanupServer(users, sr), `{"avatarUrl":null}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if len(sr.deleted) != 1 {
		t.Fatalf("expected previous avatar deleted on removal, got %v", sr.deleted)
	}
}

func TestAvatarUnchangedNoDelete(t *testing.T) {
	sameURL := avatarBase + "/users/11111111-1111-1111-1111-111111111111/avatars/same.png"
	users := &fakeUserRepo{
		byIDUser:   userWithAvatar(t, strptr(sameURL)),
		updateUser: userWithAvatar(t, strptr(sameURL)),
	}
	sr := &fakeStorageRepo{}
	patchProfile(avatarCleanupServer(users, sr), `{"avatarUrl":"`+sameURL+`"}`, true, true)
	if len(sr.deleted) != 0 {
		t.Fatalf("unchanged avatar must not delete, got %v", sr.deleted)
	}
}

func TestAvatarExternalUrlNoDelete(t *testing.T) {
	newURL := avatarBase + "/users/11111111-1111-1111-1111-111111111111/avatars/new.png"
	users := &fakeUserRepo{
		byIDUser:   userWithAvatar(t, strptr("https://external.example.org/pic.png")),
		updateUser: userWithAvatar(t, strptr(newURL)),
	}
	sr := &fakeStorageRepo{}
	patchProfile(avatarCleanupServer(users, sr), `{"avatarUrl":"`+newURL+`"}`, true, true)
	if len(sr.deleted) != 0 {
		t.Fatalf("external previous avatar must not be deleted, got %v", sr.deleted)
	}
}

func TestAvatarLegacyUploadsNoDelete(t *testing.T) {
	// A previous avatar stored under the legacy uploads/ namespace must never be
	// auto-deleted (it may be shared with post media semantics / be legacy data).
	oldURL := avatarBase + "/users/11111111-1111-1111-1111-111111111111/uploads/legacy.png"
	newURL := avatarBase + "/users/11111111-1111-1111-1111-111111111111/avatars/new.png"
	users := &fakeUserRepo{
		byIDUser:   userWithAvatar(t, strptr(oldURL)),
		updateUser: userWithAvatar(t, strptr(newURL)),
	}
	sr := &fakeStorageRepo{}
	patchProfile(avatarCleanupServer(users, sr), `{"avatarUrl":"`+newURL+`"}`, true, true)
	if len(sr.deleted) != 0 {
		t.Fatalf("legacy uploads/ avatar must not be deleted, got %v", sr.deleted)
	}
}

func TestAvatarOtherUserKeyNeverDeleted(t *testing.T) {
	// A previous avatar URL pointing at another user's namespace must never be
	// deleted, even though it is one of our public URLs.
	oldURL := avatarBase + "/users/22222222-2222-2222-2222-222222222222/avatars/foreign.png"
	newURL := avatarBase + "/users/11111111-1111-1111-1111-111111111111/avatars/new.png"
	users := &fakeUserRepo{
		byIDUser:   userWithAvatar(t, strptr(oldURL)),
		updateUser: userWithAvatar(t, strptr(newURL)),
	}
	sr := &fakeStorageRepo{}
	patchProfile(avatarCleanupServer(users, sr), `{"avatarUrl":"`+newURL+`"}`, true, true)
	if len(sr.deleted) != 0 {
		t.Fatalf("another user's key must never be deleted, got %v", sr.deleted)
	}
}

func TestGetProfileIncludesIsPrivate(t *testing.T) {
	u := userWithPassword(t, "strongpass")
	u.IsPrivate = true
	srv := profileServer(&fakeUserRepo{byIDUser: u, updateUser: u})
	rec := getProfile(srv, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"isPrivate":true`) {
		t.Fatalf("profile must expose isPrivate, got %s", rec.Body.String())
	}
}

func TestPatchProfileSetsIsPrivate(t *testing.T) {
	u := userWithPassword(t, "strongpass")
	updated := userWithPassword(t, "strongpass")
	updated.IsPrivate = true
	users := &fakeUserRepo{byIDUser: u, updateUser: updated}
	srv := profileServer(users)

	rec := patchProfile(srv, `{"isPrivate":true}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if users.lastUpdate.IsPrivate == nil || *users.lastUpdate.IsPrivate != true {
		t.Fatalf("PATCH must set IsPrivate=true in the update, got %+v", users.lastUpdate.IsPrivate)
	}
	if !strings.Contains(rec.Body.String(), `"isPrivate":true`) {
		t.Fatalf("response must reflect isPrivate:true, got %s", rec.Body.String())
	}
}
