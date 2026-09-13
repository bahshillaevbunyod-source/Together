package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"together/backend/internal/bookmark"
	"together/backend/internal/config"
	"together/backend/internal/post"
	"together/backend/internal/user"
)

// fakeBookmarkRepo is a test double for bookmark.Repository.
type fakeBookmarkRepo struct {
	saveErr     error
	removeErr   error
	isErr       error
	saved       bool
	savedSet    map[string]bool
	listErr     error
	savedItems  []bookmark.SavedItem
	savedErr    error
	saveCalls   [][2]string
	removeCalls [][2]string
}

func (f *fakeBookmarkRepo) ListSaved(_ context.Context, _ string, _ *post.Cursor, limit int) ([]bookmark.SavedItem, error) {
	if f.savedErr != nil {
		return nil, f.savedErr
	}
	if len(f.savedItems) > limit {
		return f.savedItems[:limit], nil
	}
	return f.savedItems, nil
}

func (f *fakeBookmarkRepo) ListSavedPostIDs(_ context.Context, _ string, postIDs []string) (map[string]bool, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make(map[string]bool)
	for _, id := range postIDs {
		if f.savedSet[id] {
			out[id] = true
		}
	}
	return out, nil
}

func (f *fakeBookmarkRepo) Save(_ context.Context, userID, postID string) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saveCalls = append(f.saveCalls, [2]string{userID, postID})
	return nil
}

func (f *fakeBookmarkRepo) Remove(_ context.Context, userID, postID string) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removeCalls = append(f.removeCalls, [2]string{userID, postID})
	return nil
}

func (f *fakeBookmarkRepo) IsSaved(_ context.Context, _, _ string) (bool, error) {
	return f.saved, f.isErr
}

func bookmarkServer(bm *fakeBookmarkRepo, p *post.Post) *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me, "author-id": mkUser("author-id", "author_user")}}
	return New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{is: true}, &fakeBlockRepo{},
		&fakePostRepo{getPost: p}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{}, bm, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
}

func bookmarkReq(srv *http.Server, method string, withCookie, withOrigin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/v1/posts/"+validPostID+"/bookmark", nil)
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

func publicPostBM() *post.Post { return mkPost(post.VisibilityPublic, "author-id") }

func TestSavePost(t *testing.T) {
	bm := &fakeBookmarkRepo{}
	rec := bookmarkReq(bookmarkServer(bm, publicPostBM()), http.MethodPost, true, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if len(bm.saveCalls) != 1 || bm.saveCalls[0] != [2]string{"me-id", validPostID} {
		t.Fatalf("save not recorded: %v", bm.saveCalls)
	}
}

func TestUnsavePost(t *testing.T) {
	bm := &fakeBookmarkRepo{}
	rec := bookmarkReq(bookmarkServer(bm, publicPostBM()), http.MethodDelete, true, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if len(bm.removeCalls) != 1 {
		t.Fatalf("remove not recorded")
	}
}

func TestRepeatSaveIdempotent(t *testing.T) {
	srv := bookmarkServer(&fakeBookmarkRepo{}, publicPostBM())
	_ = bookmarkReq(srv, http.MethodPost, true, true)
	rec := bookmarkReq(srv, http.MethodPost, true, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("repeat save should be 204, got %d", rec.Code)
	}
}

func TestRepeatUnsaveIdempotent(t *testing.T) {
	srv := bookmarkServer(&fakeBookmarkRepo{}, publicPostBM())
	_ = bookmarkReq(srv, http.MethodDelete, true, true)
	rec := bookmarkReq(srv, http.MethodDelete, true, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("repeat unsave should be 204, got %d", rec.Code)
	}
}

func TestBookmarkInaccessiblePost404(t *testing.T) {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	srv := New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{is: false}, &fakeBlockRepo{},
		&fakePostRepo{getPost: mkPost(post.VisibilityPrivate, "author-id")}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
	rec := bookmarkReq(srv, http.MethodPost, true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestBookmarkUnauthenticated(t *testing.T) {
	rec := bookmarkReq(bookmarkServer(&fakeBookmarkRepo{}, publicPostBM()), http.MethodPost, false, true)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestBookmarkMissingOrigin(t *testing.T) {
	rec := bookmarkReq(bookmarkServer(&fakeBookmarkRepo{}, publicPostBM()), http.MethodPost, true, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestBookmarkRepoError(t *testing.T) {
	rec := bookmarkReq(bookmarkServer(&fakeBookmarkRepo{saveErr: errForTest}, publicPostBM()), http.MethodPost, true, true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// ---- savedByMe in responses ----

func savedPostServer(bm *fakeBookmarkRepo) *http.Server {
	me := mkUser("me-id", "me_user")
	author := mkUser("author-id", "author_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me, "author-id": author}}
	return New(
		config.Config{Env: "test", Port: "8080"},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{},
		&fakePostRepo{getPost: mkPost(post.VisibilityPublic, "author-id")}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{}, bm, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
}

func decodeSavedByMe(t *testing.T, rec *httptest.ResponseRecorder) bool {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	v, _ := m["savedByMe"].(bool)
	return v
}

func TestGetPostSavedTrue(t *testing.T) {
	rec := getPost(savedPostServer(&fakeBookmarkRepo{saved: true}), validPostID, true)
	if rec.Code != http.StatusOK || !decodeSavedByMe(t, rec) {
		t.Fatalf("expected savedByMe=true, code=%d", rec.Code)
	}
}

func TestGetPostSavedFalse(t *testing.T) {
	rec := getPost(savedPostServer(&fakeBookmarkRepo{saved: false}), validPostID, true)
	if rec.Code != http.StatusOK || decodeSavedByMe(t, rec) {
		t.Fatal("expected savedByMe=false")
	}
}

func TestGetPostSavedAnonymous(t *testing.T) {
	// anonymous viewer -> savedByMe false, IsSaved never consulted
	rec := getPost(savedPostServer(&fakeBookmarkRepo{saved: true}), validPostID, false)
	if rec.Code != http.StatusOK || decodeSavedByMe(t, rec) {
		t.Fatal("anonymous must have savedByMe=false")
	}
}

func TestGetPostSavedRepoError(t *testing.T) {
	rec := getPost(savedPostServer(&fakeBookmarkRepo{isErr: errForTest}), validPostID, true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func savedFeedServer(bm *fakeBookmarkRepo, feed []post.FeedItem) *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	return New(
		config.Config{Env: "test", Port: "8080"},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{},
		&fakePostRepo{feed: feed}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{}, bm, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
}

func TestFeedSavedByMeBatch(t *testing.T) {
	feed := []post.FeedItem{
		mkFeedItem("11111111-1111-1111-1111-111111111111"),
		mkFeedItem("22222222-2222-2222-2222-222222222222"),
	}
	bm := &fakeBookmarkRepo{savedSet: map[string]bool{"11111111-1111-1111-1111-111111111111": true}}
	rec := getFeed(savedFeedServer(bm, feed), "")

	var resp struct {
		Items []struct {
			ID        string `json:"id"`
			SavedByMe bool   `json:"savedByMe"`
		} `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if !resp.Items[0].SavedByMe || resp.Items[1].SavedByMe {
		t.Fatalf("savedByMe not applied per item: %+v", resp.Items)
	}
}

func TestFeedSavedByMeRepoError(t *testing.T) {
	feed := []post.FeedItem{mkFeedItem("11111111-1111-1111-1111-111111111111")}
	bm := &fakeBookmarkRepo{listErr: errForTest}
	rec := getFeed(savedFeedServer(bm, feed), "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// ---- bookmarks list ----

func bookmarksListServer(bm *fakeBookmarkRepo) *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	return New(
		config.Config{Env: "test", Port: "8080"},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{},
		&fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{}, bm, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
}

func mkSavedItem(id string) bookmark.SavedItem {
	now := time.Now()
	return bookmark.SavedItem{
		FeedItem: post.FeedItem{
			ID: id, AuthorID: "author-id", Content: strPtr("hi"), Visibility: "public",
			CreatedAt: now, UpdatedAt: now, AuthorUsername: "author_user", AuthorDisplayName: "Author",
		},
		BookmarkedAt: now,
	}
}

func getBookmarks(srv *http.Server, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/bookmarks"+query, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func TestBookmarksList(t *testing.T) {
	bm := &fakeBookmarkRepo{savedItems: []bookmark.SavedItem{
		mkSavedItem("11111111-1111-1111-1111-111111111111"),
		mkSavedItem("22222222-2222-2222-2222-222222222222"),
	}}
	rec := getBookmarks(bookmarksListServer(bm), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp struct {
		Items []struct {
			SavedByMe bool `json:"savedByMe"`
		} `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 2 || !resp.Items[0].SavedByMe || !resp.Items[1].SavedByMe {
		t.Fatalf("expected 2 items all savedByMe=true: %+v", resp.Items)
	}
}

func TestBookmarksPagination(t *testing.T) {
	bm := &fakeBookmarkRepo{savedItems: []bookmark.SavedItem{
		mkSavedItem("11111111-1111-1111-1111-111111111111"),
		mkSavedItem("22222222-2222-2222-2222-222222222222"),
		mkSavedItem("33333333-3333-3333-3333-333333333333"),
	}}
	rec := getBookmarks(bookmarksListServer(bm), "?limit=2")
	var resp feedResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 2 || resp.NextCursor == "" {
		t.Fatalf("pagination failed: %d items, cursor %q", len(resp.Items), resp.NextCursor)
	}
	if _, ok := parseFeedCursor(resp.NextCursor); !ok {
		t.Fatal("nextCursor not decodable")
	}
}

func TestBookmarksExcludesInaccessible(t *testing.T) {
	// Exclusion happens in ListSaved's SQL; the repo returns only accessible
	// items, and the handler surfaces exactly those.
	bm := &fakeBookmarkRepo{savedItems: []bookmark.SavedItem{
		mkSavedItem("11111111-1111-1111-1111-111111111111"),
	}}
	rec := getBookmarks(bookmarksListServer(bm), "")
	var resp feedResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 1 {
		t.Fatalf("expected only accessible items, got %d", len(resp.Items))
	}
}

func TestBookmarksEmpty(t *testing.T) {
	rec := getBookmarks(bookmarksListServer(&fakeBookmarkRepo{}), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !contains2(rec.Body.String(), `"items":[]`) {
		t.Fatalf("empty list should be [], body: %s", rec.Body.String())
	}
}

func TestBookmarksUnauthenticated(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/bookmarks", nil)
	rec := httptest.NewRecorder()
	bookmarksListServer(&fakeBookmarkRepo{}).Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestBookmarksRepoError(t *testing.T) {
	rec := getBookmarks(bookmarksListServer(&fakeBookmarkRepo{savedErr: errForTest}), "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
