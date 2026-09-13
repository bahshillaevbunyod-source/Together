package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"together/backend/internal/config"
	"together/backend/internal/post"
	"together/backend/internal/session"
	"together/backend/internal/user"
)

const validPostID = "aaaaaaaa-1111-2222-3333-444444444444"

func strPtr(s string) *string { return &s }

func readServer(users *fakeUserRepo, sessions *fakeSessionRepo, follows *fakeFollowRepo, posts *fakePostRepo) *http.Server {
	return New(config.Config{Env: "test", Port: "8080"}, fakePinger{}, users, sessions, follows, &fakeBlockRepo{}, posts, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{})
}

func activeSession(userID string) *fakeSessionRepo {
	return &fakeSessionRepo{active: &session.Session{UserID: userID, ExpiresAt: time.Now().Add(time.Hour)}}
}

func mkPost(visibility, authorID string) *post.Post {
	now := time.Now()
	return &post.Post{
		ID: validPostID, AuthorID: authorID, Content: strPtr("hi"),
		Visibility: visibility, CreatedAt: now, UpdatedAt: now,
	}
}

func getPost(srv *http.Server, id string, withCookie bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+id, nil)
	if withCookie {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	}
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func TestGetPostPublicAnonymous(t *testing.T) {
	author := mkUser("author-id", "author_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"author-id": author}}
	posts := &fakePostRepo{getPost: mkPost(post.VisibilityPublic, "author-id")}
	rec := getPost(readServer(users, &fakeSessionRepo{}, &fakeFollowRepo{}, posts), validPostID, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestGetPostPrivateOther404(t *testing.T) {
	// Anonymous viewer, private post -> 404 (not 403).
	users := &fakeUserRepo{byID: map[string]*user.User{"author-id": mkUser("author-id", "author_user")}}
	posts := &fakePostRepo{getPost: mkPost(post.VisibilityPrivate, "author-id")}
	rec := getPost(readServer(users, &fakeSessionRepo{}, &fakeFollowRepo{}, posts), validPostID, false)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestGetPostPrivateOwn200(t *testing.T) {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	posts := &fakePostRepo{getPost: mkPost(post.VisibilityPrivate, "me-id")}
	rec := getPost(readServer(users, activeSession("me-id"), &fakeFollowRepo{}, posts), validPostID, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestGetPostFollowersForFollower200(t *testing.T) {
	me := mkUser("me-id", "me_user")
	author := mkUser("author-id", "author_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me, "author-id": author}}
	posts := &fakePostRepo{getPost: mkPost(post.VisibilityFollowers, "author-id")}
	rec := getPost(readServer(users, activeSession("me-id"), &fakeFollowRepo{is: true}, posts), validPostID, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestGetPostFollowersForNonFollower404(t *testing.T) {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me, "author-id": mkUser("author-id", "author_user")}}
	posts := &fakePostRepo{getPost: mkPost(post.VisibilityFollowers, "author-id")}
	rec := getPost(readServer(users, activeSession("me-id"), &fakeFollowRepo{is: false}, posts), validPostID, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestGetPostInvalidUUID(t *testing.T) {
	rec := getPost(readServer(&fakeUserRepo{}, &fakeSessionRepo{}, &fakeFollowRepo{}, &fakePostRepo{}), "not-a-uuid", false)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestGetPostNotFound(t *testing.T) {
	posts := &fakePostRepo{getPost: nil} // ErrNotFound
	rec := getPost(readServer(&fakeUserRepo{}, &fakeSessionRepo{}, &fakeFollowRepo{}, posts), validPostID, false)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestGetPostRepositoryError(t *testing.T) {
	posts := &fakePostRepo{getErr: errForTest}
	rec := getPost(readServer(&fakeUserRepo{}, &fakeSessionRepo{}, &fakeFollowRepo{}, posts), validPostID, false)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// ---- feed ----

func mkFeedItem(id string) post.FeedItem {
	now := time.Now()
	return post.FeedItem{
		ID: id, AuthorID: "author-id", Content: strPtr("hi"), Visibility: "public",
		CreatedAt: now, UpdatedAt: now, AuthorUsername: "author_user", AuthorDisplayName: "Author",
	}
}

func feedServer(posts *fakePostRepo) *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	return readServer(users, activeSession("me-id"), &fakeFollowRepo{}, posts)
}

func getFeed(srv *http.Server, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/feed"+query, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

// The feed's own/followed/public/followers inclusion and private/blocked
// exclusion are enforced by the SQL query (production). At the handler layer
// we verify it faithfully returns whatever ListFeed provides.
func TestFeedReturnsItems(t *testing.T) {
	posts := &fakePostRepo{feed: []post.FeedItem{mkFeedItem("11111111-1111-1111-1111-111111111111"), mkFeedItem("22222222-2222-2222-2222-222222222222")}}
	rec := getFeed(feedServer(posts), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp feedResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
}

func TestFeedPaginationNextCursor(t *testing.T) {
	posts := &fakePostRepo{feed: []post.FeedItem{
		mkFeedItem("11111111-1111-1111-1111-111111111111"),
		mkFeedItem("22222222-2222-2222-2222-222222222222"),
		mkFeedItem("33333333-3333-3333-3333-333333333333"),
	}}
	rec := getFeed(feedServer(posts), "?limit=2")
	var resp feedResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.NextCursor == "" {
		t.Fatal("expected a nextCursor")
	}
	if _, ok := parseFeedCursor(resp.NextCursor); !ok {
		t.Fatal("nextCursor not decodable")
	}
}

func TestFeedInvalidCursor(t *testing.T) {
	rec := getFeed(feedServer(&fakePostRepo{}), "?cursor=@@@")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestFeedInvalidLimit(t *testing.T) {
	rec := getFeed(feedServer(&fakePostRepo{}), "?limit=100")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestFeedUnauthenticated(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/feed", nil)
	rec := httptest.NewRecorder()
	feedServer(&fakePostRepo{}).Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestFeedRepositoryError(t *testing.T) {
	rec := getFeed(feedServer(&fakePostRepo{feedErr: errForTest}), "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
