package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"together/backend/internal/config"
	"together/backend/internal/storage"
	"together/backend/internal/story"
	"together/backend/internal/user"
)

// Access rules (accepted-follows only, no block, no pending requests, active
// window) are enforced and unit-tested inside the story repository SQL. These
// handler tests use a fake repository to verify the HTTP layer: auth/CSRF,
// media ownership validation on create, leak-safe error mapping, correct
// delegation (viewer id, block resolution), and that a private media URL is
// only produced for an authorized story.

// fakeStoryRepo is a test double for story.Repository.
type fakeStoryRepo struct {
	createResp *story.Story
	createErr  error
	createIn   *story.CreateInput

	gav    *story.Item
	gavErr error

	feed    []story.Item
	feedErr error

	authorItems []story.Item
	authorErr   error
	lastAuthor  [2]string // viewerID, authorID

	deleteReturn bool
	deleteErr    error
	lastDelete   [2]string // authorID, storyID

	recordReturn bool
	recordErr    error
	lastView     [2]string // storyID, viewerID
}

func (f *fakeStoryRepo) Create(_ context.Context, in story.CreateInput) (*story.Story, error) {
	f.createIn = &in
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.createResp, nil
}

func (f *fakeStoryRepo) GetByID(_ context.Context, _ string) (*story.Story, error) {
	return nil, story.ErrNotFound
}

func (f *fakeStoryRepo) GetActiveVisible(_ context.Context, _, _ string) (*story.Item, error) {
	if f.gavErr != nil {
		return nil, f.gavErr
	}
	return f.gav, nil
}

func (f *fakeStoryRepo) ListFeed(_ context.Context, _ string, _ *story.Cursor, _ int) ([]story.Item, error) {
	return f.feed, f.feedErr
}

func (f *fakeStoryRepo) ListAuthorActive(_ context.Context, viewerID, authorID string) ([]story.Item, error) {
	f.lastAuthor = [2]string{viewerID, authorID}
	return f.authorItems, f.authorErr
}

func (f *fakeStoryRepo) CanView(_ context.Context, _, _ string) (bool, error) { return true, nil }

func (f *fakeStoryRepo) DeleteOwn(_ context.Context, authorID, storyID string) (bool, error) {
	f.lastDelete = [2]string{authorID, storyID}
	return f.deleteReturn, f.deleteErr
}

func (f *fakeStoryRepo) RecordView(_ context.Context, storyID, viewerID string) (bool, error) {
	f.lastView = [2]string{storyID, viewerID}
	return f.recordReturn, f.recordErr
}

func (f *fakeStoryRepo) HasViewed(_ context.Context, _, _ string) (bool, error) { return false, nil }

// --- harness -------------------------------------------------------------

func storyServer(storageRepo *fakeStorageRepo, storyRepo *fakeStoryRepo, blocks *fakeBlockRepo, target *user.User) *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}, usernameUser: target}
	if storageRepo == nil {
		storageRepo = &fakeStorageRepo{}
	}
	if blocks == nil {
		blocks = &fakeBlockRepo{}
	}
	return New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin, MediaPublicBaseURL: "https://cdn.example.com/media"},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{}, blocks,
		&fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, storageRepo,
		&fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{},
		&fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{}, storyRepo,
	)
}

func storyReq(srv *http.Server, method, path, body string, withCookie, withOrigin bool) *httptest.ResponseRecorder {
	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	} else {
		rdr = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
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

const goodStoryID = "11111111-1111-4111-8111-111111111111"

func imageHead() *storage.ObjectInfo {
	return &storage.ObjectInfo{ContentType: "image/jpeg", SizeBytes: 1000}
}

func sampleItem(viewed bool) story.Item {
	return story.Item{
		ID:                goodStoryID,
		AuthorID:          "author-id",
		Type:              story.TypeImage,
		StorageKey:        "users/author-id/private/pic.jpg",
		MimeType:          "image/jpeg",
		CreatedAt:         time.Now(),
		AuthorUsername:    "author",
		AuthorDisplayName: "Author",
		Viewed:            viewed,
	}
}

// --- auth / CSRF ---------------------------------------------------------

func TestStoryCreateRequiresAuth(t *testing.T) {
	srv := storyServer(nil, &fakeStoryRepo{}, nil, nil)
	rec := storyReq(srv, http.MethodPost, "/api/v1/stories", `{"storageKey":"users/me-id/private/x.jpg"}`, false, true)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestStoryFeedRequiresAuth(t *testing.T) {
	srv := storyServer(nil, &fakeStoryRepo{}, nil, nil)
	rec := storyReq(srv, http.MethodGet, "/api/v1/stories", "", false, true)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestStoryCreateRequiresOrigin(t *testing.T) {
	srv := storyServer(nil, &fakeStoryRepo{}, nil, nil)
	rec := storyReq(srv, http.MethodPost, "/api/v1/stories", `{"storageKey":"users/me-id/private/x.jpg"}`, true, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 (CSRF), got %d", rec.Code)
	}
}

// --- create --------------------------------------------------------------

func TestStoryCreateAcceptsOwnPrivateUpload(t *testing.T) {
	st := &fakeStorageRepo{headInfo: imageHead()}
	key := "users/me-id/private/pic.jpg"
	repo := &fakeStoryRepo{createResp: &story.Story{
		ID: goodStoryID, AuthorID: "me-id", Type: story.TypeImage, StorageKey: key,
		MimeType: "image/jpeg", CreatedAt: time.Now(),
	}}
	srv := storyServer(st, repo, nil, nil)

	rec := storyReq(srv, http.MethodPost, "/api/v1/stories", `{"storageKey":"`+key+`"}`, true, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	if repo.createIn == nil || repo.createIn.AuthorID != "me-id" || repo.createIn.StorageKey != key || repo.createIn.Type != story.TypeImage {
		t.Fatalf("create input not derived from validated upload: %+v", repo.createIn)
	}
	body := rec.Body.String()
	// Media URL must be the presigned private URL, never a raw/public bucket URL.
	if !strings.Contains(body, "signed.example.com/private/") {
		t.Fatalf("expected presigned private media url, got %s", body)
	}
	if strings.Contains(body, "storageKey") || strings.Contains(body, "cdn.example.com") {
		t.Fatalf("response must not leak storage key or public bucket url: %s", body)
	}
	if !strings.Contains(body, `"viewed":false`) {
		t.Fatalf("new story should be unviewed: %s", body)
	}
}

func TestStoryCreateForeignKeyRejected(t *testing.T) {
	st := &fakeStorageRepo{headInfo: imageHead()}
	repo := &fakeStoryRepo{}
	srv := storyServer(st, repo, nil, nil)

	// Key belongs to another user -> ownership prefix check fails.
	rec := storyReq(srv, http.MethodPost, "/api/v1/stories", `{"storageKey":"users/other-id/private/pic.jpg"}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for foreign key, got %d", rec.Code)
	}
	if repo.createIn != nil {
		t.Fatal("must not create a story from another user's upload")
	}
}

func TestStoryCreatePublicNamespaceRejected(t *testing.T) {
	st := &fakeStorageRepo{headInfo: imageHead()}
	repo := &fakeStoryRepo{}
	srv := storyServer(st, repo, nil, nil)

	// A public "uploads" key is not allowed — stories must be private media.
	rec := storyReq(srv, http.MethodPost, "/api/v1/stories", `{"storageKey":"users/me-id/uploads/pic.jpg"}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for public-namespace key, got %d", rec.Code)
	}
	if repo.createIn != nil {
		t.Fatal("must not create a story from public-namespace media")
	}
}

func TestStoryCreateMissingObject(t *testing.T) {
	st := &fakeStorageRepo{headErr: storage.ErrObjectNotFound}
	repo := &fakeStoryRepo{}
	srv := storyServer(st, repo, nil, nil)

	rec := storyReq(srv, http.MethodPost, "/api/v1/stories", `{"storageKey":"users/me-id/private/pic.jpg"}`, true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing object, got %d", rec.Code)
	}
	if repo.createIn != nil {
		t.Fatal("must not create a story for a missing upload")
	}
}

// --- feed ----------------------------------------------------------------

func TestStoryFeedReturnsAuthorizedItems(t *testing.T) {
	repo := &fakeStoryRepo{feed: []story.Item{sampleItem(false), sampleItem(true)}}
	srv := storyServer(nil, repo, nil, nil)

	rec := storyReq(srv, http.MethodGet, "/api/v1/stories", "", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"items"`) || !strings.Contains(body, "signed.example.com/private/") {
		t.Fatalf("feed must return items with presigned media urls: %s", body)
	}
}

func TestStoryFeedInvalidCursor(t *testing.T) {
	srv := storyServer(nil, &fakeStoryRepo{}, nil, nil)
	rec := storyReq(srv, http.MethodGet, "/api/v1/stories?cursor=not-valid", "", true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid cursor, got %d", rec.Code)
	}
}

// --- single story --------------------------------------------------------

func TestGetStoryAuthorized(t *testing.T) {
	it := sampleItem(false)
	srv := storyServer(nil, &fakeStoryRepo{gav: &it}, nil, nil)
	rec := storyReq(srv, http.MethodGet, "/api/v1/stories/"+goodStoryID, "", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "signed.example.com/private/") {
		t.Fatalf("authorized story must include a presigned media url: %s", rec.Body.String())
	}
}

func TestGetStoryInaccessibleLeakSafeNoURL(t *testing.T) {
	// ErrNotFound covers nonexistent, expired, blocked, and no-access alike.
	srv := storyServer(nil, &fakeStoryRepo{gavErr: story.ErrNotFound}, nil, nil)
	rec := storyReq(srv, http.MethodGet, "/api/v1/stories/"+goodStoryID, "", true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "signed.example.com") {
		t.Fatal("no media url may be generated for an inaccessible story")
	}
}

func TestGetStoryMalformedID(t *testing.T) {
	srv := storyServer(nil, &fakeStoryRepo{}, nil, nil)
	rec := storyReq(srv, http.MethodGet, "/api/v1/stories/not-a-uuid", "", true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for malformed id, got %d", rec.Code)
	}
}

// --- user's stories ------------------------------------------------------

func TestUserStoriesReturnsActive(t *testing.T) {
	target := mkUser("author-id", "author")
	it := sampleItem(false)
	repo := &fakeStoryRepo{authorItems: []story.Item{it}}
	srv := storyServer(nil, repo, nil, target)

	rec := storyReq(srv, http.MethodGet, "/api/v1/users/author/stories", "", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if repo.lastAuthor != [2]string{"me-id", "author-id"} {
		t.Fatalf("must query author-active as (viewer, author): %v", repo.lastAuthor)
	}
}

func TestUserStoriesNoAccessReturnsEmpty(t *testing.T) {
	// Repository returns no rows when the viewer lacks access (non-follower /
	// pending request); the handler must return an empty list, never leak.
	target := mkUser("author-id", "author")
	repo := &fakeStoryRepo{authorItems: nil}
	srv := storyServer(nil, repo, nil, target)

	rec := storyReq(srv, http.MethodGet, "/api/v1/users/author/stories", "", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 empty, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("no-access must yield an empty list: %s", rec.Body.String())
	}
}

func TestUserStoriesBlockedNotFound(t *testing.T) {
	target := mkUser("author-id", "author")
	repo := &fakeStoryRepo{authorItems: []story.Item{sampleItem(false)}}
	srv := storyServer(nil, repo, &fakeBlockRepo{hasBetween: true}, target)

	rec := storyReq(srv, http.MethodGet, "/api/v1/users/author/stories", "", true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("blocked viewer must get 404, got %d", rec.Code)
	}
}

func TestUserStoriesUnknownUser(t *testing.T) {
	srv := storyServer(nil, &fakeStoryRepo{}, nil, nil) // usernameUser nil -> ErrNotFound
	rec := storyReq(srv, http.MethodGet, "/api/v1/users/ghost/stories", "", true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown user, got %d", rec.Code)
	}
}

// --- view ----------------------------------------------------------------

func TestViewStoryRecordsWhenAuthorized(t *testing.T) {
	it := sampleItem(false)
	repo := &fakeStoryRepo{gav: &it, recordReturn: true}
	srv := storyServer(nil, repo, nil, nil)

	rec := storyReq(srv, http.MethodPost, "/api/v1/stories/"+goodStoryID+"/view", "", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if repo.lastView != [2]string{goodStoryID, "me-id"} {
		t.Fatalf("view must be recorded as (story, viewer): %v", repo.lastView)
	}
}

func TestViewStoryIdempotent(t *testing.T) {
	it := sampleItem(false)
	// recordReturn false simulates a repeat view (ON CONFLICT DO NOTHING).
	repo := &fakeStoryRepo{gav: &it, recordReturn: false}
	srv := storyServer(nil, repo, nil, nil)

	rec := storyReq(srv, http.MethodPost, "/api/v1/stories/"+goodStoryID+"/view", "", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("repeat view should still be 200, got %d", rec.Code)
	}
}

func TestViewStoryInaccessibleNotRecorded(t *testing.T) {
	repo := &fakeStoryRepo{gavErr: story.ErrNotFound}
	srv := storyServer(nil, repo, nil, nil)

	rec := storyReq(srv, http.MethodPost, "/api/v1/stories/"+goodStoryID+"/view", "", true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if repo.lastView != [2]string{"", ""} {
		t.Fatalf("no view may be recorded for an inaccessible story: %v", repo.lastView)
	}
}

func TestViewStoryRequiresOrigin(t *testing.T) {
	it := sampleItem(false)
	srv := storyServer(nil, &fakeStoryRepo{gav: &it}, nil, nil)
	rec := storyReq(srv, http.MethodPost, "/api/v1/stories/"+goodStoryID+"/view", "", true, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 (CSRF), got %d", rec.Code)
	}
}

// --- delete --------------------------------------------------------------

func TestDeleteStoryOwnerSucceeds(t *testing.T) {
	repo := &fakeStoryRepo{deleteReturn: true}
	srv := storyServer(nil, repo, nil, nil)

	rec := storyReq(srv, http.MethodDelete, "/api/v1/stories/"+goodStoryID, "", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if repo.lastDelete != [2]string{"me-id", goodStoryID} {
		t.Fatalf("delete must be scoped to (owner, story): %v", repo.lastDelete)
	}
}

func TestDeleteStoryNonOwnerNotFound(t *testing.T) {
	// DeleteOwn deletes nothing for a non-owner -> 404, never revealing the story.
	repo := &fakeStoryRepo{deleteReturn: false}
	srv := storyServer(nil, repo, nil, nil)

	rec := storyReq(srv, http.MethodDelete, "/api/v1/stories/"+goodStoryID, "", true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-owner/nonexistent, got %d", rec.Code)
	}
}

func TestDeleteStoryMalformedID(t *testing.T) {
	srv := storyServer(nil, &fakeStoryRepo{}, nil, nil)
	rec := storyReq(srv, http.MethodDelete, "/api/v1/stories/nope", "", true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for malformed id, got %d", rec.Code)
	}
}
