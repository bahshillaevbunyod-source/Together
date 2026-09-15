package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"together/backend/internal/config"
	"together/backend/internal/media"
	"together/backend/internal/post"
	"together/backend/internal/user"
)

func discoverPostsServer(posts *fakePostRepo, mediaRepo *fakeMediaRepo, bookmarks *fakeBookmarkRepo) *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	if mediaRepo == nil {
		mediaRepo = &fakeMediaRepo{}
	}
	if bookmarks == nil {
		bookmarks = &fakeBookmarkRepo{}
	}
	return New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin, MediaPublicBaseURL: "https://cdn.example.com/media"},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{},
		posts, &fakeLikeRepo{}, &fakeCommentRepo{}, mediaRepo, &fakeStorageRepo{}, bookmarks, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
}

func doDiscoverPosts(srv *http.Server, rawQuery string, withCookie bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts/discover"+rawQuery, nil)
	if withCookie {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	}
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

type discPostsResp struct {
	Items []struct {
		ID     string `json:"id"`
		Author struct {
			ID string `json:"id"`
		} `json:"author"`
		Media         []json.RawMessage `json:"media"`
		LikesCount    int64             `json:"likesCount"`
		CommentsCount int64             `json:"commentsCount"`
		LikedByMe     bool              `json:"likedByMe"`
	} `json:"items"`
	NextCursor string `json:"nextCursor"`
}

func decodeDiscPosts(t *testing.T, rec *httptest.ResponseRecorder) discPostsResp {
	t.Helper()
	var resp discPostsResp
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v (%s)", err, rec.Body.String())
	}
	return resp
}

func discPostIDs(resp discPostsResp) []string {
	out := make([]string, len(resp.Items))
	for i, it := range resp.Items {
		out[i] = it.ID
	}
	return out
}

func mkDiscItem(id, authorID, visibility string, createdAt time.Time) post.FeedItem {
	return post.FeedItem{
		ID: id, AuthorID: authorID, Content: strPtr("hello"), Visibility: visibility,
		CreatedAt: createdAt, UpdatedAt: createdAt,
		AuthorUsername: "u_" + authorID, AuthorDisplayName: "User " + authorID,
	}
}

func TestDiscoverPostsIncludesPublicExcludesRest(t *testing.T) {
	base := time.Now()
	posts := &fakePostRepo{
		discoverPool: []post.FeedItem{
			mkDiscItem("p1", "author-a", "public", base),                        // included
			mkDiscItem("p2", "author-b", "followers", base.Add(-1*time.Minute)), // excluded: visibility
			mkDiscItem("p3", "author-c", "private", base.Add(-2*time.Minute)),   // excluded: visibility
			mkDiscItem("p4", "me-id", "public", base.Add(-3*time.Minute)),       // excluded: self
			mkDiscItem("p5", "author-d", "public", base.Add(-4*time.Minute)),    // excluded: followed
			mkDiscItem("p6", "author-e", "public", base.Add(-5*time.Minute)),    // excluded: blocked
		},
		discoverFollowed: map[string]bool{"author-d": true},
		discoverBlocked:  map[string]bool{"author-e": true},
	}
	rec := doDiscoverPosts(discoverPostsServer(posts, nil, nil), "", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	got := discPostIDs(decodeDiscPosts(t, rec))
	if len(got) != 1 || got[0] != "p1" {
		t.Fatalf("expected only [p1], got %v", got)
	}
}

func TestDiscoverPostsMediaAndCounts(t *testing.T) {
	base := time.Now()
	item := mkDiscItem("p1", "author-a", "public", base)
	item.LikesCount = 5
	item.CommentsCount = 3
	item.LikedByMe = true
	posts := &fakePostRepo{discoverPool: []post.FeedItem{item}}
	mediaRepo := &fakeMediaRepo{batch: map[string][]media.Media{
		"p1": {{ID: "m1", PostID: "p1", Type: media.TypeImage, StorageKey: "a.webp", MimeType: "image/webp", SortOrder: 0}},
	}}
	rec := doDiscoverPosts(discoverPostsServer(posts, mediaRepo, nil), "", true)
	resp := decodeDiscPosts(t, rec)
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	it := resp.Items[0]
	if it.LikesCount != 5 || it.CommentsCount != 3 || !it.LikedByMe {
		t.Fatalf("counts not mapped: %+v", it)
	}
	if len(it.Media) != 1 {
		t.Fatalf("expected 1 media, got %d", len(it.Media))
	}
	if it.Author.ID != "author-a" {
		t.Fatalf("author not mapped: %q", it.Author.ID)
	}
}

func TestDiscoverPostsKeysetPagination(t *testing.T) {
	base := time.Now()
	pool := []post.FeedItem{
		mkDiscItem("11111111-1111-1111-1111-111111111111", "a1", "public", base),
		mkDiscItem("22222222-2222-2222-2222-222222222222", "a2", "public", base.Add(-1*time.Minute)),
		mkDiscItem("33333333-3333-3333-3333-333333333333", "a3", "public", base.Add(-2*time.Minute)),
		mkDiscItem("44444444-4444-4444-4444-444444444444", "a4", "public", base.Add(-3*time.Minute)),
		mkDiscItem("55555555-5555-5555-5555-555555555555", "a5", "public", base.Add(-4*time.Minute)),
	}
	posts := &fakePostRepo{discoverPool: pool}
	srv := discoverPostsServer(posts, nil, nil)

	var seen []string
	cursor := ""
	for page := 0; page < 10; page++ {
		q := "?limit=2"
		if cursor != "" {
			q += "&cursor=" + cursor
		}
		resp := decodeDiscPosts(t, doDiscoverPosts(srv, q, true))
		if len(resp.Items) == 0 {
			t.Fatalf("page %d unexpectedly empty", page)
		}
		if len(resp.Items) > 2 {
			t.Fatalf("page %d exceeded limit: %d", page, len(resp.Items))
		}
		seen = append(seen, discPostIDs(resp)...)
		if resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
	}

	want := []string{
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
		"33333333-3333-3333-3333-333333333333",
		"44444444-4444-4444-4444-444444444444",
		"55555555-5555-5555-5555-555555555555",
	}
	if len(seen) != len(want) {
		t.Fatalf("expected %v, got %v", want, seen)
	}
	uniq := map[string]bool{}
	for i, id := range seen {
		if id != want[i] {
			t.Fatalf("order wrong: expected %v, got %v", want, seen)
		}
		if uniq[id] {
			t.Fatalf("duplicate across pages: %s (%v)", id, seen)
		}
		uniq[id] = true
	}
}

func TestDiscoverPostsMalformedCursor(t *testing.T) {
	rec := doDiscoverPosts(discoverPostsServer(&fakePostRepo{}, nil, nil), "?cursor=@@@bad", true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDiscoverPostsRejectsCursorWithInvalidUUID(t *testing.T) {
	cursor := encodeCursor(time.Now(), "not-a-uuid")
	rec := doDiscoverPosts(discoverPostsServer(&fakePostRepo{}, nil, nil), "?cursor="+cursor, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDiscoverPostsInvalidLimit(t *testing.T) {
	if rec := doDiscoverPosts(discoverPostsServer(&fakePostRepo{}, nil, nil), "?limit=abc", true); rec.Code != http.StatusBadRequest {
		t.Fatalf("non-numeric limit: expected 400, got %d", rec.Code)
	}
	if rec := doDiscoverPosts(discoverPostsServer(&fakePostRepo{}, nil, nil), "?limit=0", true); rec.Code != http.StatusBadRequest {
		t.Fatalf("zero limit: expected 400, got %d", rec.Code)
	}
}

func TestDiscoverPostsLimitCap(t *testing.T) {
	posts := &fakePostRepo{}
	doDiscoverPosts(discoverPostsServer(posts, nil, nil), "?limit=500", true)
	// Handler caps to 30 and fetches limit+1 to detect the next page.
	if posts.lastDiscoverLimit != discoverPostsMaxLimit+1 {
		t.Fatalf("expected repo limit %d, got %d", discoverPostsMaxLimit+1, posts.lastDiscoverLimit)
	}
}

func TestDiscoverPostsUnauthenticated(t *testing.T) {
	rec := doDiscoverPosts(discoverPostsServer(&fakePostRepo{}, nil, nil), "", false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestDiscoverPostsRepoError(t *testing.T) {
	posts := &fakePostRepo{discoverErr: errForTest}
	rec := doDiscoverPosts(discoverPostsServer(posts, nil, nil), "", true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
