package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"together/backend/internal/config"
	"together/backend/internal/media"
	"together/backend/internal/post"
	"together/backend/internal/storage"
	"together/backend/internal/user"
)

// fakePostRepo is a test double for post.Repository.
type fakePostRepo struct {
	createErr error
	last      post.CreateInput
	getPost   *post.Post
	getErr    error
	feed      []post.FeedItem
	feedErr   error
	updateErr error
	deleteErr error
	deleted   []string
}

func (f *fakePostRepo) Update(_ context.Context, id string, in post.PostUpdate) (*post.Post, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	p := *f.getPost
	p.ID = id
	if in.Content != nil {
		p.Content = in.Content
	}
	if in.Visibility != nil {
		p.Visibility = *in.Visibility
	}
	p.UpdatedAt = time.Now()
	return &p, nil
}

func (f *fakePostRepo) Delete(_ context.Context, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *fakePostRepo) Create(_ context.Context, in post.CreateInput) (*post.Post, error) {
	f.last = in
	if f.createErr != nil {
		return nil, f.createErr
	}
	now := time.Now()
	return &post.Post{
		ID:         "post-1",
		AuthorID:   in.AuthorID,
		Content:    in.Content,
		Visibility: in.Visibility,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

func (f *fakePostRepo) GetByID(_ context.Context, _ string) (*post.Post, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.getPost == nil {
		return nil, post.ErrNotFound
	}
	return f.getPost, nil
}

func (f *fakePostRepo) ListFeed(_ context.Context, _ string, _ *post.Cursor, limit int) ([]post.FeedItem, error) {
	if f.feedErr != nil {
		return nil, f.feedErr
	}
	if len(f.feed) > limit {
		return f.feed[:limit], nil
	}
	return f.feed, nil
}

// fakePostCreate is a test double for postWithMediaCreator.
type fakePostCreate struct {
	created   *post.Post
	err       error
	lastIn    post.CreateInput
	lastMedia []media.CreateInput
}

func (f *fakePostCreate) CreatePostWithMedia(_ context.Context, in post.CreateInput, items []media.CreateInput) (*post.Post, error) {
	f.lastIn = in
	f.lastMedia = items
	if f.err != nil {
		return nil, f.err
	}
	if f.created != nil {
		return f.created, nil
	}
	return &post.Post{ID: "post-1", AuthorID: in.AuthorID, Content: in.Content, Visibility: in.Visibility, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
}

func createServer(pc *fakePostCreate, sr *fakeStorageRepo) *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	return New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin, MediaPublicBaseURL: "https://cdn.example.com/media"},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{},
		&fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, sr, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, pc, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
}

func createPost(srv *http.Server, body string, withCookie, withOrigin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewBufferString(body))
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

func okStorage() *fakeStorageRepo {
	return &fakeStorageRepo{headInfo: &storage.ObjectInfo{ContentType: "image/png", SizeBytes: 1000}}
}

const mediaKey = "users/me-id/uploads/x.png"

func TestCreatePostTextOnly(t *testing.T) {
	pc := &fakePostCreate{}
	rec := createPost(createServer(pc, &fakeStorageRepo{}), `{"content":"hello","visibility":"public"}`, true, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	if pc.lastIn.Content == nil || *pc.lastIn.Content != "hello" || len(pc.lastMedia) != 0 {
		t.Fatalf("unexpected create input: %+v media=%v", pc.lastIn, pc.lastMedia)
	}
}

func TestCreatePostMediaOnly(t *testing.T) {
	pc := &fakePostCreate{}
	rec := createPost(createServer(pc, okStorage()), `{"storageKeys":["`+mediaKey+`"]}`, true, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	if pc.lastIn.Content != nil {
		t.Fatalf("media-only post must have nil content, got %v", *pc.lastIn.Content)
	}
	if len(pc.lastMedia) != 1 || pc.lastMedia[0].SortOrder != 0 || pc.lastMedia[0].Type != "image" {
		t.Fatalf("unexpected media: %+v", pc.lastMedia)
	}
}

func TestCreatePostTextPlusMedia(t *testing.T) {
	pc := &fakePostCreate{}
	rec := createPost(createServer(pc, okStorage()), `{"content":"hi","storageKeys":["`+mediaKey+`"]}`, true, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if pc.lastIn.Content == nil || len(pc.lastMedia) != 1 {
		t.Fatalf("expected text+media")
	}
}

func TestCreatePostEmpty(t *testing.T) {
	rec := createPost(createServer(&fakePostCreate{}, &fakeStorageRepo{}), `{}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreatePostTooManyMedia(t *testing.T) {
	// 9 keys exceeds the product limit of 8.
	keys := ""
	for i := 0; i < 9; i++ {
		if i > 0 {
			keys += ","
		}
		keys += `"users/me-id/uploads/k` + string(rune('a'+i)) + `.png"`
	}
	rec := createPost(createServer(&fakePostCreate{}, okStorage()), `{"storageKeys":[`+keys+`]}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreatePostMaxMedia(t *testing.T) {
	// Exactly 8 distinct keys is the maximum and must be accepted, preserving
	// selection order as sort_order.
	pc := &fakePostCreate{}
	keys := ""
	for i := 0; i < 8; i++ {
		if i > 0 {
			keys += ","
		}
		keys += `"users/me-id/uploads/k` + string(rune('a'+i)) + `.png"`
	}
	rec := createPost(createServer(pc, okStorage()), `{"storageKeys":[`+keys+`]}`, true, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	if len(pc.lastMedia) != 8 {
		t.Fatalf("expected 8 media, got %d", len(pc.lastMedia))
	}
	for i, m := range pc.lastMedia {
		if m.SortOrder != i {
			t.Fatalf("expected sortOrder %d, got %d", i, m.SortOrder)
		}
	}
}

func TestCreatePostDuplicateKey(t *testing.T) {
	rec := createPost(createServer(&fakePostCreate{}, okStorage()), `{"storageKeys":["`+mediaKey+`","`+mediaKey+`"]}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreatePostForeignKey(t *testing.T) {
	rec := createPost(createServer(&fakePostCreate{}, okStorage()), `{"storageKeys":["users/other/uploads/x.png"]}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreatePostMissingObject(t *testing.T) {
	sr := &fakeStorageRepo{headErr: storage.ErrObjectNotFound}
	rec := createPost(createServer(&fakePostCreate{}, sr), `{"storageKeys":["`+mediaKey+`"]}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreatePostStorageFailure(t *testing.T) {
	sr := &fakeStorageRepo{headErr: errForTest}
	rec := createPost(createServer(&fakePostCreate{}, sr), `{"storageKeys":["`+mediaKey+`"]}`, true, true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestCreatePostMediaInsertFailure(t *testing.T) {
	// atomic create returns an error -> nothing saved -> 500
	pc := &fakePostCreate{err: errForTest}
	rec := createPost(createServer(pc, okStorage()), `{"content":"hi","storageKeys":["`+mediaKey+`"]}`, true, true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestCreatePostDefaultVisibility(t *testing.T) {
	pc := &fakePostCreate{}
	rec := createPost(createServer(pc, &fakeStorageRepo{}), `{"content":"hi"}`, true, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if pc.lastIn.Visibility != "public" {
		t.Fatalf("expected default visibility public, got %q", pc.lastIn.Visibility)
	}
}

func TestCreatePostTooLong(t *testing.T) {
	rec := createPost(createServer(&fakePostCreate{}, &fakeStorageRepo{}), `{"content":"`+strings.Repeat("a", 5001)+`"}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreatePostInvalidVisibility(t *testing.T) {
	rec := createPost(createServer(&fakePostCreate{}, &fakeStorageRepo{}), `{"content":"hi","visibility":"secret"}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreatePostUnauthenticated(t *testing.T) {
	rec := createPost(createServer(&fakePostCreate{}, &fakeStorageRepo{}), `{"content":"hi"}`, false, true)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCreatePostMissingOrigin(t *testing.T) {
	rec := createPost(createServer(&fakePostCreate{}, &fakeStorageRepo{}), `{"content":"hi"}`, true, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestCreatePostAuthorFromContext(t *testing.T) {
	pc := &fakePostCreate{}
	rec := createPost(createServer(pc, &fakeStorageRepo{}), `{"content":"hi"}`, true, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if pc.lastIn.AuthorID != "me-id" {
		t.Fatalf("author_id must come from auth context, got %q", pc.lastIn.AuthorID)
	}
	if strings.Contains(rec.Body.String(), "password") || strings.Contains(rec.Body.String(), "email") {
		t.Fatalf("response leaked private fields: %s", rec.Body.String())
	}
}

func TestCreatePostMediaConflict(t *testing.T) {
	// storage_key already attached to another post -> 409
	pc := &fakePostCreate{err: media.ErrMediaAlreadyAttached}
	rec := createPost(createServer(pc, okStorage()), `{"storageKeys":["`+mediaKey+`"]}`, true, true)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != `{"error":"media already attached"}` {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}
