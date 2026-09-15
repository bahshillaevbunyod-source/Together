package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"together/backend/internal/auth"
	"together/backend/internal/block"
	"together/backend/internal/config"
	"together/backend/internal/follow"
	"together/backend/internal/post"
	"together/backend/internal/session"
	"together/backend/internal/user"
)

// fakeUserRepo is a test double for user.Repository. Never used in production.
type fakeUserRepo struct {
	err          error
	last         user.CreateInput
	getUser      *user.User
	getErr       error
	byIDUser     *user.User
	byIDErr      error
	byID         map[string]*user.User
	updateUser   *user.User
	updateErr    error
	lastUpdate   user.ProfileUpdate
	usernameUser *user.User
	usernameErr  error
	lastUsername string
}

func (f *fakeUserRepo) Create(_ context.Context, in user.CreateInput) (*user.User, error) {
	f.last = in
	if f.err != nil {
		return nil, f.err
	}
	email := in.Email
	now := time.Now()
	return &user.User{
		ID:             "11111111-1111-1111-1111-111111111111",
		Email:          &email,
		Username:       in.Username,
		DisplayName:    in.DisplayName,
		NativeLanguage: in.NativeLanguage,
		PasswordHash:   &in.PasswordHash,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func (f *fakeUserRepo) GetByEmail(_ context.Context, _ string) (*user.User, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.getUser == nil {
		return nil, user.ErrNotFound
	}
	return f.getUser, nil
}

func (f *fakeUserRepo) GetByID(_ context.Context, id string) (*user.User, error) {
	if f.byIDErr != nil {
		return nil, f.byIDErr
	}
	if f.byID != nil {
		if u, ok := f.byID[id]; ok {
			return u, nil
		}
		return nil, user.ErrNotFound
	}
	if f.byIDUser == nil {
		return nil, user.ErrNotFound
	}
	return f.byIDUser, nil
}

func (f *fakeUserRepo) GetByUsername(_ context.Context, username string) (*user.User, error) {
	f.lastUsername = username
	if f.usernameErr != nil {
		return nil, f.usernameErr
	}
	if f.usernameUser == nil {
		return nil, user.ErrNotFound
	}
	return f.usernameUser, nil
}

func (f *fakeUserRepo) UpdateProfile(_ context.Context, _ string, in user.ProfileUpdate) (*user.User, error) {
	f.lastUpdate = in
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	if f.updateUser != nil {
		return f.updateUser, nil
	}
	return f.byIDUser, nil
}

// fakeSessionRepo is a test double for session.Repository.
type fakeSessionRepo struct {
	created   *session.Session
	createErr error
	active    *session.Session
	activeErr error
	deleted   []string
	deleteErr error
}

func (f *fakeSessionRepo) Create(_ context.Context, userID, tokenHash string, expiresAt time.Time) (*session.Session, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = &session.Session{
		ID:        "22222222-2222-2222-2222-222222222222",
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
	return f.created, nil
}

func (f *fakeSessionRepo) GetActiveByTokenHash(_ context.Context, _ string) (*session.Session, error) {
	if f.activeErr != nil {
		return nil, f.activeErr
	}
	if f.active == nil {
		return nil, session.ErrNotFound
	}
	return f.active, nil
}

func (f *fakeSessionRepo) DeleteByTokenHash(_ context.Context, tokenHash string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, tokenHash)
	return nil
}

func registerServer(repo user.Repository) *http.Server {
	return buildServer(config.Config{Env: "test", Port: "8080"}, repo, &fakeSessionRepo{})
}

func buildServer(cfg config.Config, users user.Repository, sessions session.Repository) *http.Server {
	return New(cfg, fakePinger{}, users, sessions, &fakeFollowRepo{}, &fakeBlockRepo{}, &fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{})
}

func buildServerWithFollows(cfg config.Config, users user.Repository, sessions session.Repository, follows follow.Repository) *http.Server {
	return New(cfg, fakePinger{}, users, sessions, follows, &fakeBlockRepo{}, &fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{follows: follows}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{})
}

func buildServerWithBlocks(cfg config.Config, users user.Repository, sessions session.Repository, blocks block.Repository) *http.Server {
	return New(cfg, fakePinger{}, users, sessions, &fakeFollowRepo{}, blocks, &fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{})
}

func buildServerFull(cfg config.Config, users user.Repository, sessions session.Repository, follows follow.Repository, blocks block.Repository) *http.Server {
	return New(cfg, fakePinger{}, users, sessions, follows, blocks, &fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{follows: follows}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{})
}

func buildServerWithStorage(cfg config.Config, users user.Repository, sessions session.Repository, storageRepo *fakeStorageRepo) *http.Server {
	return New(cfg, fakePinger{}, users, sessions, &fakeFollowRepo{}, &fakeBlockRepo{}, &fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, storageRepo, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{})
}

func buildServerWithPosts(cfg config.Config, users user.Repository, sessions session.Repository, posts post.Repository) *http.Server {
	return New(cfg, fakePinger{}, users, sessions, &fakeFollowRepo{}, &fakeBlockRepo{}, posts, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{})
}

func sessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			return c
		}
	}
	return nil
}

func postRegister(t *testing.T, srv *http.Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	return postJSON(t, srv, "/api/v1/auth/register", body)
}

func postJSON(t *testing.T, srv *http.Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

// userWithPassword builds a stored user whose password_hash matches password.
func userWithPassword(t *testing.T, password string) *user.User {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash setup failed: %v", err)
	}
	email := "alex@example.com"
	now := time.Now()
	return &user.User{
		ID:             "11111111-1111-1111-1111-111111111111",
		Email:          &email,
		Username:       "alex_01",
		DisplayName:    "Alex",
		NativeLanguage: "en",
		PasswordHash:   &hash,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

const loginBody = `{"email":"Alex@Example.com","password":"strongpass"}`

func TestLoginSuccess(t *testing.T) {
	repo := &fakeUserRepo{getUser: userWithPassword(t, "strongpass")}
	rec := postJSON(t, registerServer(repo), "/api/v1/auth/login", loginBody)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Fatalf("response leaked password data: %s", rec.Body.String())
	}
}

func TestLoginUnknownEmail(t *testing.T) {
	repo := &fakeUserRepo{getUser: nil} // GetByEmail -> ErrNotFound
	rec := postJSON(t, registerServer(repo), "/api/v1/auth/login", loginBody)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	repo := &fakeUserRepo{getUser: userWithPassword(t, "strongpass")}
	body := `{"email":"alex@example.com","password":"wrongpass"}`
	rec := postJSON(t, registerServer(repo), "/api/v1/auth/login", body)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestLoginNoPasswordHash(t *testing.T) {
	u := userWithPassword(t, "strongpass")
	u.PasswordHash = nil
	repo := &fakeUserRepo{getUser: u}
	rec := postJSON(t, registerServer(repo), "/api/v1/auth/login", loginBody)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestLoginRepositoryError(t *testing.T) {
	repo := &fakeUserRepo{getErr: context.DeadlineExceeded}
	rec := postJSON(t, registerServer(repo), "/api/v1/auth/login", loginBody)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "deadline") {
		t.Fatalf("internal error leaked details: %s", rec.Body.String())
	}
}

func TestLoginInvalidJSON(t *testing.T) {
	rec := postJSON(t, registerServer(&fakeUserRepo{}), "/api/v1/auth/login", `{"email":`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestLoginCreatesSessionAndSetsCookie(t *testing.T) {
	users := &fakeUserRepo{getUser: userWithPassword(t, "strongpass")}
	sessions := &fakeSessionRepo{}
	srv := buildServer(config.Config{Env: "test", Port: "8080"}, users, sessions)

	rec := postJSON(t, srv, "/api/v1/auth/login", loginBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if sessions.created == nil {
		t.Fatal("no session was created")
	}
	cookie := sessionCookie(rec)
	if cookie == nil || cookie.Value == "" {
		t.Fatal("session cookie was not set")
	}
	// The DB must store only the hash — never the raw token.
	if sessions.created.TokenHash == cookie.Value {
		t.Fatal("stored token hash equals the raw cookie token")
	}
	if sessions.created.TokenHash != session.HashToken(cookie.Value) {
		t.Fatal("stored token hash does not match hash of the cookie token")
	}
}

func TestLoginCookieAttributes(t *testing.T) {
	users := &fakeUserRepo{getUser: userWithPassword(t, "strongpass")}
	srv := buildServer(config.Config{Env: "test", Port: "8080"}, users, &fakeSessionRepo{})

	rec := postJSON(t, srv, "/api/v1/auth/login", loginBody)
	cookie := sessionCookie(rec)
	if cookie == nil {
		t.Fatal("no session cookie")
	}
	if !cookie.HttpOnly {
		t.Error("cookie must be HttpOnly")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie SameSite must be Lax, got %v", cookie.SameSite)
	}
	if cookie.Path != "/" {
		t.Errorf("cookie Path must be /, got %q", cookie.Path)
	}
	if cookie.Secure {
		t.Error("cookie must not be Secure outside production")
	}
}

func TestLoginProductionCookieSecure(t *testing.T) {
	users := &fakeUserRepo{getUser: userWithPassword(t, "strongpass")}
	srv := buildServer(config.Config{Env: "production", Port: "8080"}, users, &fakeSessionRepo{})

	rec := postJSON(t, srv, "/api/v1/auth/login", loginBody)
	cookie := sessionCookie(rec)
	if cookie == nil {
		t.Fatal("no session cookie")
	}
	if !cookie.Secure {
		t.Error("cookie must be Secure in production")
	}
}

func TestLogoutDeletesSessionAndClearsCookie(t *testing.T) {
	sessions := &fakeSessionRepo{}
	srv := buildServer(config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin}, &fakeUserRepo{}, sessions)

	rawToken := "raw-token-value"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Origin", testOrigin)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: rawToken})
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(sessions.deleted) != 1 || sessions.deleted[0] != session.HashToken(rawToken) {
		t.Fatalf("expected session deleted by token hash, got %v", sessions.deleted)
	}
	cookie := sessionCookie(rec)
	if cookie == nil || cookie.MaxAge >= 0 {
		t.Fatalf("expected cleared cookie (MaxAge < 0), got %+v", cookie)
	}
}

func getMe(srv *http.Server, cookieValue string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	if cookieValue != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookieValue})
	}
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func TestMeValidSession(t *testing.T) {
	users := &fakeUserRepo{byIDUser: userWithPassword(t, "strongpass")}
	sessions := &fakeSessionRepo{active: &session.Session{
		ID: "s1", UserID: "u1", TokenHash: "h", ExpiresAt: time.Now().Add(time.Hour),
	}}
	srv := buildServer(config.Config{Env: "test", Port: "8080"}, users, sessions)

	rec := getMe(srv, "raw-token")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Fatalf("response leaked password data: %s", rec.Body.String())
	}
}

func TestMeMissingCookie(t *testing.T) {
	srv := buildServer(config.Config{Env: "test", Port: "8080"}, &fakeUserRepo{}, &fakeSessionRepo{})
	if rec := getMe(srv, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMeUnknownToken(t *testing.T) {
	// active == nil -> ErrNotFound
	srv := buildServer(config.Config{Env: "test", Port: "8080"}, &fakeUserRepo{}, &fakeSessionRepo{})
	if rec := getMe(srv, "raw-token"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMeExpiredSession(t *testing.T) {
	// The production query filters expired sessions, so the repo returns
	// ErrNotFound — modeled here as active == nil.
	srv := buildServer(config.Config{Env: "test", Port: "8080"}, &fakeUserRepo{}, &fakeSessionRepo{active: nil})
	if rec := getMe(srv, "raw-token"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMeUserNotFound(t *testing.T) {
	sessions := &fakeSessionRepo{active: &session.Session{UserID: "ghost", ExpiresAt: time.Now().Add(time.Hour)}}
	srv := buildServer(config.Config{Env: "test", Port: "8080"}, &fakeUserRepo{byIDUser: nil}, sessions)
	if rec := getMe(srv, "raw-token"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMeInternalError(t *testing.T) {
	sessions := &fakeSessionRepo{activeErr: context.DeadlineExceeded}
	srv := buildServer(config.Config{Env: "test", Port: "8080"}, &fakeUserRepo{}, sessions)
	rec := getMe(srv, "raw-token")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "deadline") {
		t.Fatalf("internal error leaked details: %s", rec.Body.String())
	}
}

func TestLogoutMissingCookieNoError(t *testing.T) {
	srv := buildServer(config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin}, &fakeUserRepo{}, &fakeSessionRepo{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Origin", testOrigin)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("missing cookie must not cause 500, got %d", rec.Code)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if c := sessionCookie(rec); c == nil || c.MaxAge >= 0 {
		t.Fatal("logout should clear the cookie even when none was sent")
	}
}

const validBody = `{"email":"Alex@Example.com","username":"alex_01","displayName":"Alex","nativeLanguage":"en","password":"strongpass"}`

func TestRegisterSuccess(t *testing.T) {
	repo := &fakeUserRepo{}
	rec := postRegister(t, registerServer(repo), validBody)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	// email + username must be normalized to lowercase before persisting.
	if repo.last.Email != "alex@example.com" {
		t.Fatalf("email not normalized: %q", repo.last.Email)
	}
	if repo.last.Username != "alex_01" {
		t.Fatalf("username not normalized: %q", repo.last.Username)
	}
	// password must be hashed, never stored as plaintext.
	if repo.last.PasswordHash == "strongpass" || repo.last.PasswordHash == "" {
		t.Fatalf("password was not hashed: %q", repo.last.PasswordHash)
	}

	// response must not leak the password hash.
	raw := rec.Body.String()
	if strings.Contains(raw, "password") || strings.Contains(raw, repo.last.PasswordHash) {
		t.Fatalf("response leaked password data: %s", raw)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body["id"] == "" || body["username"] != "alex_01" {
		t.Fatalf("unexpected response body: %s", raw)
	}
}

func TestRegisterInvalidUsername(t *testing.T) {
	body := `{"email":"a@b.com","username":"Bad Name!","displayName":"A","nativeLanguage":"en","password":"strongpass"}`
	rec := postRegister(t, registerServer(&fakeUserRepo{}), body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestRegisterWeakPassword(t *testing.T) {
	body := `{"email":"a@b.com","username":"alex_01","displayName":"A","nativeLanguage":"en","password":"short"}`
	rec := postRegister(t, registerServer(&fakeUserRepo{}), body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestRegisterDuplicate(t *testing.T) {
	rec := postRegister(t, registerServer(&fakeUserRepo{err: user.ErrDuplicateEmail}), validBody)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestRegisterInternalError(t *testing.T) {
	rec := postRegister(t, registerServer(&fakeUserRepo{err: context.DeadlineExceeded}), validBody)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	// internal errors must not leak details.
	if strings.Contains(rec.Body.String(), "deadline") {
		t.Fatalf("internal error leaked details: %s", rec.Body.String())
	}
}

// --- registration auto-login (STEP: register creates a session) ---

func TestRegisterSetsSessionCookie(t *testing.T) {
	users := &fakeUserRepo{}
	sessions := &fakeSessionRepo{}
	srv := buildServer(config.Config{Env: "test", Port: "8080"}, users, sessions)

	rec := postRegister(t, srv, validBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	if sessions.created == nil {
		t.Fatal("register did not create a session")
	}
	cookie := sessionCookie(rec)
	if cookie == nil || cookie.Value == "" {
		t.Fatal("register did not set the session cookie")
	}
	// DB stores only the hash of the raw cookie token.
	if sessions.created.TokenHash == cookie.Value {
		t.Fatal("stored token hash equals the raw cookie token")
	}
	if sessions.created.TokenHash != session.HashToken(cookie.Value) {
		t.Fatal("stored token hash does not match hash of the cookie token")
	}
}

func TestRegisterCookieAuthenticatesMe(t *testing.T) {
	users := &fakeUserRepo{}
	sessions := &fakeSessionRepo{}
	srv := buildServer(config.Config{Env: "test", Port: "8080"}, users, sessions)

	rec := postRegister(t, srv, validBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	cookie := sessionCookie(rec)
	if cookie == nil {
		t.Fatal("register did not set the session cookie")
	}

	// Wire the store so the just-issued cookie resolves to an active session + user.
	sessions.active = sessions.created
	users.byIDUser = &user.User{
		ID:             sessions.created.UserID,
		Username:       "alex_01",
		DisplayName:    "Alex",
		NativeLanguage: "en",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie.Value})
	me := httptest.NewRecorder()
	srv.Handler.ServeHTTP(me, req)

	if me.Code != http.StatusOK {
		t.Fatalf("register cookie should authenticate /me, got %d (%s)", me.Code, me.Body.String())
	}
}
