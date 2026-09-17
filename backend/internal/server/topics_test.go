package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"together/backend/internal/config"
	"together/backend/internal/post"
	"together/backend/internal/topic"
	"together/backend/internal/user"
)

type fakeTopicRepo struct {
	bySlug             map[string]*topic.Topic
	getErr             error
	trending           []topic.TrendingItem
	trendingErr        error
	search             []topic.TrendingItem
	searchErr          error
	lastSlug           string
	lastViewerID       *string
	lastLimit          int
	lastSearchQuery    string
	lastSearchViewerID *string
	lastSearchLimit    int
}

func (f *fakeTopicRepo) CreatePostTopicsTx(context.Context, topic.DBTX, string, []string) error {
	return nil
}

func (f *fakeTopicRepo) GetBySlug(_ context.Context, slug string) (*topic.Topic, error) {
	f.lastSlug = slug
	if f.getErr != nil {
		return nil, f.getErr
	}
	item := f.bySlug[slug]
	if item == nil {
		return nil, topic.ErrNotFound
	}
	return item, nil
}

func (f *fakeTopicRepo) ListTrending(_ context.Context, viewerID *string, limit int) ([]topic.TrendingItem, error) {
	f.lastViewerID = viewerID
	f.lastLimit = limit
	if f.trendingErr != nil {
		return nil, f.trendingErr
	}
	if len(f.trending) > limit {
		return f.trending[:limit], nil
	}
	return f.trending, nil
}

func (f *fakeTopicRepo) Search(_ context.Context, viewerID *string, query string, limit int) ([]topic.TrendingItem, error) {
	f.lastSearchViewerID = viewerID
	f.lastSearchQuery = query
	f.lastSearchLimit = limit
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	if len(f.search) > limit {
		return f.search[:limit], nil
	}
	return f.search, nil
}

func topicsServer(topics *fakeTopicRepo, posts *fakePostRepo, sessions *fakeSessionRepo) *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	return New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users, sessions, &fakeFollowRepo{}, &fakeBlockRepo{},
		posts, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{}, topics,
	)
}

func doTopicsRequest(srv *http.Server, path string, withCookie bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if withCookie {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	}
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func testTopic() *topic.Topic {
	return &topic.Topic{ID: "11111111-1111-1111-1111-111111111111", Slug: "travel", CreatedAt: time.Now()}
}

func TestListTopicsReturnsTrendingForAnonymousViewer(t *testing.T) {
	topics := &fakeTopicRepo{trending: []topic.TrendingItem{{Slug: "travel", PostsCount: 8}}}
	rec := doTopicsRequest(topicsServer(topics, &fakePostRepo{}, &fakeSessionRepo{}), "/api/v1/topics", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if topics.lastViewerID != nil {
		t.Fatal("anonymous topics request must use a nil viewer")
	}
	var response trendingTopicsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if len(response.Items) != 1 || response.Items[0].Slug != "travel" || response.Items[0].PostsCount != 8 {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestSearchTopicsCanonicalizesQueryAndUsesSmallLimit(t *testing.T) {
	topics := &fakeTopicRepo{search: []topic.TrendingItem{{Slug: "travel", PostsCount: 8}}}
	rec := doTopicsRequest(topicsServer(topics, &fakePostRepo{}, &fakeSessionRepo{}), "/api/v1/topics/search?q=%23TRAVEL", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if topics.lastSearchQuery != "travel" || topics.lastSearchViewerID != nil || topics.lastSearchLimit != topicSearchLimit {
		t.Fatalf("unexpected search call: query=%q viewer=%v limit=%d", topics.lastSearchQuery, topics.lastSearchViewerID, topics.lastSearchLimit)
	}
	var response trendingTopicsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if len(response.Items) != 1 || response.Items[0].Slug != "travel" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestSearchTopicsRejectsInvalidQueryAndHandlesRepositoryError(t *testing.T) {
	if rec := doTopicsRequest(topicsServer(&fakeTopicRepo{}, &fakePostRepo{}, &fakeSessionRepo{}), "/api/v1/topics/search?q=", false); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty query: expected 400, got %d", rec.Code)
	}
	topics := &fakeTopicRepo{searchErr: errors.New("boom")}
	if rec := doTopicsRequest(topicsServer(topics, &fakePostRepo{}, &fakeSessionRepo{}), "/api/v1/topics/search?q=travel", false); rec.Code != http.StatusInternalServerError {
		t.Fatalf("repository error: expected 500, got %d", rec.Code)
	}
}

func TestGetTopicCanonicalizesSlugAndRequiresVisiblePost(t *testing.T) {
	topicItem := testTopic()
	topics := &fakeTopicRepo{bySlug: map[string]*topic.Topic{"travel": topicItem}}
	posts := &fakePostRepo{topicPosts: []post.FeedItem{mkFeedItem("11111111-1111-1111-1111-111111111111")}}
	rec := doTopicsRequest(topicsServer(topics, posts, &fakeSessionRepo{}), "/api/v1/topics/TRAVEL", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if topics.lastSlug != "travel" || posts.lastTopicID != topicItem.ID || posts.lastTopicViewer != nil {
		t.Fatalf("lookup did not use canonical anonymous access: slug=%q topic=%q viewer=%v", topics.lastSlug, posts.lastTopicID, posts.lastTopicViewer)
	}
}

func TestTopicPostsUseFeedShapeAndKeysetPagination(t *testing.T) {
	base := time.Now()
	topicItem := testTopic()
	topics := &fakeTopicRepo{bySlug: map[string]*topic.Topic{"travel": topicItem}}
	posts := &fakePostRepo{topicPosts: []post.FeedItem{
		mkDiscItem("11111111-1111-1111-1111-111111111111", "author-a", "public", base),
		mkDiscItem("22222222-2222-2222-2222-222222222222", "author-b", "public", base.Add(-time.Minute)),
		mkDiscItem("33333333-3333-3333-3333-333333333333", "author-c", "public", base.Add(-2*time.Minute)),
	}}
	srv := topicsServer(topics, posts, activeSession("me-id"))
	rec := doTopicsRequest(srv, "/api/v1/topics/travel/posts?limit=2", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var first feedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if len(first.Items) != 2 || first.NextCursor == "" || first.Items[0].ID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("unexpected first page: %+v", first)
	}
	rec = doTopicsRequest(srv, "/api/v1/topics/travel/posts?limit=2&cursor="+first.NextCursor, true)
	var second feedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &second); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].ID != "33333333-3333-3333-3333-333333333333" || second.NextCursor != "" {
		t.Fatalf("unexpected second page: %+v", second)
	}
}

func TestTopicPostsRejectInvalidCursorAndHideInaccessibleTopic(t *testing.T) {
	topicItem := testTopic()
	topics := &fakeTopicRepo{bySlug: map[string]*topic.Topic{"travel": topicItem}}
	srv := topicsServer(topics, &fakePostRepo{topicPosts: []post.FeedItem{mkFeedItem("11111111-1111-1111-1111-111111111111")}}, &fakeSessionRepo{})
	if rec := doTopicsRequest(srv, "/api/v1/topics/travel/posts?cursor=bad", false); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid cursor: expected 400, got %d", rec.Code)
	}
	if rec := doTopicsRequest(topicsServer(topics, &fakePostRepo{}, &fakeSessionRepo{}), "/api/v1/topics/travel/posts", false); rec.Code != http.StatusNotFound {
		t.Fatalf("hidden topic: expected 404, got %d", rec.Code)
	}
}

func TestTopicsRepositoryErrorsAreInternal(t *testing.T) {
	topics := &fakeTopicRepo{getErr: errors.New("boom")}
	if rec := doTopicsRequest(topicsServer(topics, &fakePostRepo{}, &fakeSessionRepo{}), "/api/v1/topics/travel", false); rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
