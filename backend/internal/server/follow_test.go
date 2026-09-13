package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"together/backend/internal/config"
	"together/backend/internal/follow"
	"together/backend/internal/session"
	"together/backend/internal/user"
)

// fakeFollowRepo is a test double for follow.Repository.
type fakeFollowRepo struct {
	followErr     error
	unfollowErr   error
	isErr         error
	is            bool
	followers     int64
	following     int64
	countErr      error
	followersList []follow.ListItem
	followingList []follow.ListItem
	listErr       error
	followed      [][2]string
	unfollowed    [][2]string
}

func (f *fakeFollowRepo) ListFollowers(_ context.Context, _ string, _ *follow.Cursor, limit int) ([]follow.ListItem, error) {
	return truncate(f.followersList, f.listErr, limit)
}

func (f *fakeFollowRepo) ListFollowing(_ context.Context, _ string, _ *follow.Cursor, limit int) ([]follow.ListItem, error) {
	return truncate(f.followingList, f.listErr, limit)
}

func truncate(items []follow.ListItem, err error, limit int) ([]follow.ListItem, error) {
	if err != nil {
		return nil, err
	}
	if len(items) > limit {
		return items[:limit], nil
	}
	return items, nil
}

func (f *fakeFollowRepo) CountFollowers(_ context.Context, _ string) (int64, error) {
	return f.followers, f.countErr
}

func (f *fakeFollowRepo) CountFollowing(_ context.Context, _ string) (int64, error) {
	return f.following, f.countErr
}

func (f *fakeFollowRepo) Follow(_ context.Context, a, b string) error {
	if f.followErr != nil {
		return f.followErr
	}
	f.followed = append(f.followed, [2]string{a, b})
	return nil
}

func (f *fakeFollowRepo) Unfollow(_ context.Context, a, b string) error {
	if f.unfollowErr != nil {
		return f.unfollowErr
	}
	f.unfollowed = append(f.unfollowed, [2]string{a, b})
	return nil
}

func (f *fakeFollowRepo) IsFollowing(_ context.Context, _, _ string) (bool, error) {
	return f.is, f.isErr
}

// fakeFollowNotifier is a test double for the atomic follow+notify service. It
// delegates to an optional fakeFollowRepo so repository errors still surface,
// and records the (follower, following) pairs it was asked to follow.
type fakeFollowNotifier struct {
	follows follow.Repository
	err     error
	calls   [][2]string
}

func (f *fakeFollowNotifier) Follow(ctx context.Context, followerID, followingID string) error {
	f.calls = append(f.calls, [2]string{followerID, followingID})
	if f.err != nil {
		return f.err
	}
	if f.follows != nil {
		return f.follows.Follow(ctx, followerID, followingID)
	}
	return nil
}

func mkUser(id, username string) *user.User {
	return &user.User{
		ID: id, Username: username, DisplayName: username,
		NativeLanguage: "en", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
}

// follow server: me (via GetByID) and target (via GetByUsername).
func followSetup(t *testing.T, target *user.User, follows *fakeFollowRepo) *http.Server {
	t.Helper()
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byIDUser: me, usernameUser: target}
	return buildServerWithFollows(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		users,
		&fakeSessionRepo{active: &session.Session{UserID: "me-id", ExpiresAt: time.Now().Add(time.Hour)}},
		follows,
	)
}

func followReq(srv *http.Server, method, username string, withCookie, withOrigin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/v1/users/"+username+"/follow", nil)
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

func TestFollowSuccess(t *testing.T) {
	srv := followSetup(t, mkUser("target-id", "target_user"), &fakeFollowRepo{})
	rec := followReq(srv, http.MethodPost, "target_user", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"following":true`) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestFollowDuplicateIdempotent(t *testing.T) {
	srv := followSetup(t, mkUser("target-id", "target_user"), &fakeFollowRepo{})
	_ = followReq(srv, http.MethodPost, "target_user", true, true)
	rec := followReq(srv, http.MethodPost, "target_user", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("duplicate follow should be 200, got %d", rec.Code)
	}
}

func TestUnfollowSuccess(t *testing.T) {
	srv := followSetup(t, mkUser("target-id", "target_user"), &fakeFollowRepo{})
	rec := followReq(srv, http.MethodDelete, "target_user", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"following":false`) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestUnfollowWhenNotFollowing(t *testing.T) {
	srv := followSetup(t, mkUser("target-id", "target_user"), &fakeFollowRepo{})
	rec := followReq(srv, http.MethodDelete, "target_user", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("unfollow when not following should be 200, got %d", rec.Code)
	}
}

func TestFollowSelf(t *testing.T) {
	// target resolves to the same user as the authenticated user.
	me := mkUser("me-id", "me_user")
	srv := followSetup(t, me, &fakeFollowRepo{})
	rec := followReq(srv, http.MethodPost, "me_user", true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("self-follow should be 400, got %d", rec.Code)
	}
}

func TestFollowUnknownTarget(t *testing.T) {
	srv := followSetup(t, nil, &fakeFollowRepo{}) // GetByUsername -> ErrNotFound
	rec := followReq(srv, http.MethodPost, "ghost", true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown target should be 404, got %d", rec.Code)
	}
}

func TestFollowUnauthenticated(t *testing.T) {
	srv := followSetup(t, mkUser("target-id", "target_user"), &fakeFollowRepo{})
	rec := followReq(srv, http.MethodPost, "target_user", false, true)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestFollowMissingOrigin(t *testing.T) {
	srv := followSetup(t, mkUser("target-id", "target_user"), &fakeFollowRepo{})
	rec := followReq(srv, http.MethodPost, "target_user", true, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestFollowRepositoryError(t *testing.T) {
	srv := followSetup(t, mkUser("target-id", "target_user"), &fakeFollowRepo{followErr: errForTest})
	rec := followReq(srv, http.MethodPost, "target_user", true, true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// followNotifySetup builds a follow server using the given notifier, so we can
// observe whether the handler invokes the atomic follow+notify use-case.
func followNotifySetup(t *testing.T, target *user.User, notifier *fakeFollowNotifier) *http.Server {
	t.Helper()
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byIDUser: me, usernameUser: target}
	return New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users,
		&fakeSessionRepo{active: &session.Session{UserID: "me-id", ExpiresAt: time.Now().Add(time.Hour)}},
		&fakeFollowRepo{}, &fakeBlockRepo{}, &fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{},
		&fakeMediaRepo{}, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{},
		&fakePostCreate{}, notifier, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
}

func TestFollowInvokesNotifier(t *testing.T) {
	notifier := &fakeFollowNotifier{}
	srv := followNotifySetup(t, mkUser("target-id", "target_user"), notifier)
	rec := followReq(srv, http.MethodPost, "target_user", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(notifier.calls) != 1 || notifier.calls[0] != [2]string{"me-id", "target-id"} {
		t.Fatalf("expected follow(me-id, target-id) via notifier, got %v", notifier.calls)
	}
}

func TestFollowSelfSkipsNotifier(t *testing.T) {
	notifier := &fakeFollowNotifier{}
	me := mkUser("me-id", "me_user")
	srv := followNotifySetup(t, me, notifier)
	rec := followReq(srv, http.MethodPost, "me_user", true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("self-follow should be 400, got %d", rec.Code)
	}
	if len(notifier.calls) != 0 {
		t.Fatal("self-follow must not reach the notifier")
	}
}

func TestFollowBlockedSkipsNotifier(t *testing.T) {
	notifier := &fakeFollowNotifier{}
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byIDUser: me, usernameUser: mkUser("target-id", "target_user")}
	srv := New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users,
		&fakeSessionRepo{active: &session.Session{UserID: "me-id", ExpiresAt: time.Now().Add(time.Hour)}},
		&fakeFollowRepo{}, &fakeBlockRepo{hasBetween: true}, &fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{},
		&fakeMediaRepo{}, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{},
		&fakePostCreate{}, notifier, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
	rec := followReq(srv, http.MethodPost, "target_user", true, true)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("blocked follow should be 403, got %d", rec.Code)
	}
	if len(notifier.calls) != 0 {
		t.Fatal("blocked follow must not reach the notifier")
	}
}
