package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"together/backend/internal/auth"
	"together/backend/internal/block"
	"together/backend/internal/config"
	"together/backend/internal/follow"
	"together/backend/internal/post"
	"together/backend/internal/registration"
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

	// Search test controls.
	searchPool      []*user.User    // candidate users to match against
	searchBlocked   map[string]bool // user ids in a block relationship with the viewer
	searchErr       error
	lastSearchQuery string
	lastSearchLimit int
	searchCalled    bool

	// Discover test controls.
	discoverPool   []*user.User     // candidate users
	followerCounts map[string]int64 // user id -> follower count
	followedByMe   map[string]bool  // ids the viewer already follows (excluded)
	discoverErr    error
	lastDiscover   user.DiscoverParams
	discoverCalled bool
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

// SearchUsers is a faithful in-memory mirror of the SQL contract in
// user.PostgresRepository.SearchUsers: case-insensitive substring match on
// username or display name, excluding the viewer and any blocked user, ranked
// (exact username, username prefix, display-name prefix, contains) with stable
// username+id tie-breaks, capped at limit. The parameterized SQL is the source
// of truth; this lets handler tests exercise the documented behavior.
func (f *fakeUserRepo) SearchUsers(_ context.Context, viewerID, query string, limit int) ([]user.SearchResult, error) {
	f.searchCalled = true
	f.lastSearchQuery = query
	f.lastSearchLimit = limit
	if f.searchErr != nil {
		return nil, f.searchErr
	}

	q := strings.ToLower(strings.TrimSpace(query))

	type ranked struct {
		u    *user.User
		rank int
	}
	var matches []ranked
	for _, u := range f.searchPool {
		if u.ID == viewerID {
			continue // never return self
		}
		if f.searchBlocked[u.ID] {
			continue // block relationship in either direction
		}
		uname := strings.ToLower(u.Username)
		dname := strings.ToLower(u.DisplayName)
		unameMatch := strings.Contains(uname, q)
		dnameMatch := strings.Contains(dname, q)
		if !unameMatch && !dnameMatch {
			continue
		}
		rank := 3
		switch {
		case uname == q:
			rank = 0
		case strings.HasPrefix(uname, q):
			rank = 1
		case strings.HasPrefix(dname, q):
			rank = 2
		}
		matches = append(matches, ranked{u: u, rank: rank})
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].rank != matches[j].rank {
			return matches[i].rank < matches[j].rank
		}
		if matches[i].u.Username != matches[j].u.Username {
			return matches[i].u.Username < matches[j].u.Username
		}
		return matches[i].u.ID < matches[j].u.ID
	})

	out := make([]user.SearchResult, 0, len(matches))
	for _, m := range matches {
		if len(out) >= limit {
			break
		}
		out = append(out, user.SearchResult{
			ID:          m.u.ID,
			Username:    m.u.Username,
			DisplayName: m.u.DisplayName,
			AvatarURL:   m.u.AvatarURL,
		})
	}
	return out, nil
}

// DiscoverCountries mirrors user.PostgresRepository.DiscoverCountries: the
// DiscoverUsers exclusions, grouped by country, count DESC then code ASC.
func (f *fakeUserRepo) DiscoverCountries(_ context.Context, viewerID string) ([]user.CountryCount, error) {
	if f.discoverErr != nil {
		return nil, f.discoverErr
	}
	counts := map[string]int64{}
	for _, u := range f.discoverPool {
		if u.ID == viewerID || f.searchBlocked[u.ID] || f.followedByMe[u.ID] || u.CountryCode == nil {
			continue
		}
		counts[*u.CountryCode]++
	}
	out := []user.CountryCount{}
	for code, n := range counts {
		out = append(out, user.CountryCount{CountryCode: code, People: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].People != out[j].People {
			return out[i].People > out[j].People
		}
		return out[i].CountryCode < out[j].CountryCode
	})
	return out, nil
}

// DiscoverUsers mirrors the SQL contract in user.PostgresRepository.DiscoverUsers:
// excludes self, blocked ids and already-followed ids; applies the world country
// filter; ranks deterministically per mode; then offset/limit paginates. The
// parameterized SQL is the source of truth; this drives handler tests.
func (f *fakeUserRepo) DiscoverUsers(_ context.Context, p user.DiscoverParams) ([]user.DiscoverResult, error) {
	f.discoverCalled = true
	f.lastDiscover = p
	if f.discoverErr != nil {
		return nil, f.discoverErr
	}

	deref := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}

	var cand []*user.User
	for _, u := range f.discoverPool {
		if u.ID == p.ViewerID || f.searchBlocked[u.ID] || f.followedByMe[u.ID] {
			continue
		}
		if p.Mode == user.DiscoverWorld && p.Country != "" && deref(u.CountryCode) != p.Country {
			continue
		}
		cand = append(cand, u)
	}

	fc := func(id string) int64 { return f.followerCounts[id] }
	// score mirrors the SQL for_you score: follower_count + light boosts.
	score := func(u *user.User) int64 {
		s := fc(u.ID)
		if p.ViewerCountry != "" && deref(u.CountryCode) == p.ViewerCountry {
			s += user.ForYouCountryBoost
		}
		if p.ViewerLanguage != "" && u.NativeLanguage == p.ViewerLanguage {
			s += user.ForYouLanguageBoost
		}
		return s
	}

	sort.SliceStable(cand, func(i, j int) bool {
		a, b := cand[i], cand[j]
		switch p.Mode {
		case user.DiscoverPopular:
			if fc(a.ID) != fc(b.ID) {
				return fc(a.ID) > fc(b.ID)
			}
			return a.ID > b.ID
		case user.DiscoverWorld:
			ra, rb := worldRank(deref(a.CountryCode), p.ViewerCountry), worldRank(deref(b.CountryCode), p.ViewerCountry)
			if ra != rb {
				return ra < rb
			}
			if fc(a.ID) != fc(b.ID) {
				return fc(a.ID) > fc(b.ID)
			}
			return a.ID > b.ID
		default: // for_you
			if score(a) != score(b) {
				return score(a) > score(b)
			}
			if !a.CreatedAt.Equal(b.CreatedAt) {
				return a.CreatedAt.After(b.CreatedAt)
			}
			return a.ID > b.ID
		}
	})

	// Keyset pagination: start strictly after the cursor row (found by id in the
	// fully-ordered list — the ordering is total, so this equals the SQL keyset).
	start := 0
	if p.After != nil {
		for i, u := range cand {
			if u.ID == p.After.ID {
				start = i + 1
				break
			}
		}
	}
	if start >= len(cand) {
		return nil, nil
	}
	end := start + p.Limit
	if end > len(cand) {
		end = len(cand)
	}
	page := cand[start:end]

	out := make([]user.DiscoverResult, 0, len(page))
	for _, u := range page {
		out = append(out, user.DiscoverResult{
			ID:             u.ID,
			Username:       u.Username,
			DisplayName:    u.DisplayName,
			AvatarURL:      u.AvatarURL,
			CountryCode:    u.CountryCode,
			City:           u.City,
			NativeLanguage: u.NativeLanguage,
			FollowerCount:  fc(u.ID),
			CreatedAt:      u.CreatedAt,
			Score:          score(u),
			CountryRank:    worldRank(deref(u.CountryCode), p.ViewerCountry),
		})
	}
	return out, nil
}

// worldRank mirrors the world ORDER BY: known-different country first (0), same
// country (1), unknown country last (2).
func worldRank(country, viewerCountry string) int {
	switch {
	case country == "":
		return 2
	case country == viewerCountry:
		return 1
	default:
		return 0
	}
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

type fakeRegistrationCreator struct {
	users    user.Repository
	sessions session.Repository
}

func (f fakeRegistrationCreator) Register(ctx context.Context, in user.CreateInput, tokenHash string, expiresAt time.Time) (*user.User, error) {
	created, err := f.users.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	if _, err := f.sessions.Create(ctx, created.ID, tokenHash, expiresAt); err != nil {
		return nil, err
	}
	return created, nil
}

var _ registration.Creator = fakeRegistrationCreator{}

func buildServer(cfg config.Config, users user.Repository, sessions session.Repository) *http.Server {
	return New(cfg, fakePinger{}, users, sessions, &fakeFollowRepo{}, &fakeBlockRepo{}, &fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{}, fakeRegistrationCreator{users: users, sessions: sessions})
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

func TestRegisterProfileFieldsUseProfileValidationLimits(t *testing.T) {
	tests := []struct {
		name        string
		displayName string
		language    string
		want        int
	}{
		{name: "unicode boundary accepted", displayName: strings.Repeat("界", 80), language: "uz", want: http.StatusCreated},
		{name: "display name too long", displayName: strings.Repeat("x", 81), language: "en", want: http.StatusBadRequest},
		{name: "native language too long by runes", displayName: "Alex", language: strings.Repeat("界", 17), want: http.StatusBadRequest},
		{name: "native language too short", displayName: "Alex", language: "e", want: http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"email":"a@b.com","username":"alex_01","displayName":"` + tc.displayName + `","nativeLanguage":"` + tc.language + `","password":"strongpass"}`
			rec := postRegister(t, registerServer(&fakeUserRepo{}), body)
			if rec.Code != tc.want {
				t.Fatalf("expected %d, got %d (%s)", tc.want, rec.Code, rec.Body.String())
			}
		})
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
