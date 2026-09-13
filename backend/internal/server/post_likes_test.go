package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"together/backend/internal/config"
	"together/backend/internal/like"
	"together/backend/internal/post"
	"together/backend/internal/user"
)

// fakeLikeRepo is a test double for like.Repository.
type fakeLikeRepo struct {
	likeErr     error
	unlikeErr   error
	isErr       error
	countErr    error
	count       int64
	liked       bool
	likeCalls   [][2]string
	unlikeCalls [][2]string
}

func (f *fakeLikeRepo) LikePost(_ context.Context, userID, postID string) error {
	if f.likeErr != nil {
		return f.likeErr
	}
	f.likeCalls = append(f.likeCalls, [2]string{userID, postID})
	return nil
}

func (f *fakeLikeRepo) UnlikePost(_ context.Context, userID, postID string) error {
	if f.unlikeErr != nil {
		return f.unlikeErr
	}
	f.unlikeCalls = append(f.unlikeCalls, [2]string{userID, postID})
	return nil
}

func (f *fakeLikeRepo) IsPostLiked(_ context.Context, _, _ string) (bool, error) {
	return f.liked, f.isErr
}

func (f *fakeLikeRepo) CountPostLikes(_ context.Context, _ string) (int64, error) {
	return f.count, f.countErr
}

// fakeLikeNotifier is a test double for the atomic like+notify service. It
// delegates to an optional like repo so repository errors still surface, and
// records the (actor, post, author) triples it was asked to like.
type fakeLikeNotifier struct {
	likes like.Repository
	err   error
	calls [][3]string
}

func (f *fakeLikeNotifier) Like(ctx context.Context, actorID, postID, authorID string) error {
	f.calls = append(f.calls, [3]string{actorID, postID, authorID})
	if f.err != nil {
		return f.err
	}
	if f.likes != nil {
		return f.likes.LikePost(ctx, actorID, postID)
	}
	return nil
}

func readServerLikes(users *fakeUserRepo, sessions *fakeSessionRepo, follows *fakeFollowRepo, posts *fakePostRepo, likes like.Repository) *http.Server {
	return New(config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin}, fakePinger{}, users, sessions, follows, &fakeBlockRepo{}, posts, likes, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{likes: likes}, &fakeCommentNotifier{}, &fakeConversationRepo{})
}

// server where me can see a public post, with the given like repo.
func likeServer(likes *fakeLikeRepo, p *post.Post) *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me, "author-id": mkUser("author-id", "author_user")}}
	return readServerLikes(users, activeSession("me-id"), &fakeFollowRepo{is: true}, &fakePostRepo{getPost: p}, likes)
}

func likeReq(srv *http.Server, method string, withCookie, withOrigin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/v1/posts/"+validPostID+"/like", nil)
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

func decodeLike(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	return m
}

func TestLikeSuccess(t *testing.T) {
	srv := likeServer(&fakeLikeRepo{count: 5}, mkPost(post.VisibilityPublic, "author-id"))
	rec := likeReq(srv, http.MethodPost, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	m := decodeLike(t, rec)
	if m["liked"] != true || m["likesCount"].(float64) != 5 {
		t.Fatalf("unexpected body: %v", m)
	}
}

func TestLikeDuplicateIdempotent(t *testing.T) {
	srv := likeServer(&fakeLikeRepo{count: 1}, mkPost(post.VisibilityPublic, "author-id"))
	_ = likeReq(srv, http.MethodPost, true, true)
	rec := likeReq(srv, http.MethodPost, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("duplicate like should be 200, got %d", rec.Code)
	}
}

func TestUnlikeSuccess(t *testing.T) {
	srv := likeServer(&fakeLikeRepo{count: 0}, mkPost(post.VisibilityPublic, "author-id"))
	rec := likeReq(srv, http.MethodDelete, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if decodeLike(t, rec)["liked"] != false {
		t.Fatal("expected liked=false")
	}
}

func TestUnlikeWhenNotLikedIdempotent(t *testing.T) {
	srv := likeServer(&fakeLikeRepo{count: 0}, mkPost(post.VisibilityPublic, "author-id"))
	rec := likeReq(srv, http.MethodDelete, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestLikeInaccessiblePost404(t *testing.T) {
	// private post owned by someone else, viewer me -> not visible -> 404
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	srv := readServerLikes(users, activeSession("me-id"), &fakeFollowRepo{is: false},
		&fakePostRepo{getPost: mkPost(post.VisibilityPrivate, "author-id")}, &fakeLikeRepo{})
	rec := likeReq(srv, http.MethodPost, true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestLikeUnauthenticated(t *testing.T) {
	srv := likeServer(&fakeLikeRepo{}, mkPost(post.VisibilityPublic, "author-id"))
	rec := likeReq(srv, http.MethodPost, false, true)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestLikeMissingOrigin(t *testing.T) {
	srv := likeServer(&fakeLikeRepo{}, mkPost(post.VisibilityPublic, "author-id"))
	rec := likeReq(srv, http.MethodPost, true, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestLikeCountUpdated(t *testing.T) {
	srv := likeServer(&fakeLikeRepo{count: 42}, mkPost(post.VisibilityPublic, "author-id"))
	rec := likeReq(srv, http.MethodPost, true, true)
	if decodeLike(t, rec)["likesCount"].(float64) != 42 {
		t.Fatal("likesCount not reflected in response")
	}
}

func TestLikeRepositoryError(t *testing.T) {
	srv := likeServer(&fakeLikeRepo{likeErr: errForTest}, mkPost(post.VisibilityPublic, "author-id"))
	rec := likeReq(srv, http.MethodPost, true, true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// blockedAccessServer: viewer me, a public post by author-id, with a block
// between them.
func blockedAccessServer() *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me, "author-id": mkUser("author-id", "author_user")}}
	return New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{},
		&fakeBlockRepo{hasBetween: true},
		&fakePostRepo{getPost: mkPost(post.VisibilityPublic, "author-id")},
		&fakeLikeRepo{},
		&fakeCommentRepo{},
		&fakeMediaRepo{},
		&fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
}

func TestLikeBlockedPost404(t *testing.T) {
	rec := likeReq(blockedAccessServer(), http.MethodPost, true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("blocked like should be 404, got %d", rec.Code)
	}
	// must not reveal that a block was the reason
	if rec.Body.String() != "" && decodeLike(t, rec)["error"] != "post not found" {
		t.Fatalf("response should not reveal block: %s", rec.Body.String())
	}
}

func TestGetPostBlocked404(t *testing.T) {
	rec := getPost(blockedAccessServer(), validPostID, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("blocked read should be 404, got %d", rec.Code)
	}
}

func TestGetPostIncludesLikes(t *testing.T) {
	me := mkUser("me-id", "me_user")
	author := mkUser("author-id", "author_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me, "author-id": author}}
	posts := &fakePostRepo{getPost: mkPost(post.VisibilityPublic, "author-id")}
	srv := readServerLikes(users, activeSession("me-id"), &fakeFollowRepo{}, posts, &fakeLikeRepo{count: 3, liked: true})

	rec := getPost(srv, validPostID, true)
	m := decodeLike(t, rec)
	if m["likesCount"].(float64) != 3 || m["likedByMe"] != true {
		t.Fatalf("post response missing likes: %v", m)
	}
}

func TestFeedIncludesLikes(t *testing.T) {
	item := mkFeedItem("11111111-1111-1111-1111-111111111111")
	item.LikesCount = 7
	item.LikedByMe = true
	rec := getFeed(feedServer(&fakePostRepo{feed: []post.FeedItem{item}}), "")

	var resp feedResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 1 || resp.Items[0].LikesCount != 7 || !resp.Items[0].LikedByMe {
		t.Fatalf("feed item missing likes: %+v", resp.Items)
	}
}
