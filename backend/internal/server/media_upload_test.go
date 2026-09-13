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

	"together/backend/internal/config"
	"together/backend/internal/storage"
	"together/backend/internal/user"
)

// fakeStorageRepo is a test double for storage.Repository.
type fakeStorageRepo struct {
	createErr error
	lastKey   string
	lastMime  string
	lastSize  int64
	url       string
	deleteErr error
	headInfo  *storage.ObjectInfo
	headErr   error
}

func (f *fakeStorageRepo) CreateUploadURL(_ context.Context, key, mimeType string, sizeBytes int64) (*storage.PresignedUpload, error) {
	f.lastKey = key
	f.lastMime = mimeType
	f.lastSize = sizeBytes
	if f.createErr != nil {
		return nil, f.createErr
	}
	u := f.url
	if u == "" {
		u = "https://storage.example.com/put"
	}
	return &storage.PresignedUpload{UploadURL: u, ExpiresAt: time.Now().Add(10 * time.Minute)}, nil
}

func (f *fakeStorageRepo) DeleteObject(_ context.Context, _ string) error {
	return f.deleteErr
}

func (f *fakeStorageRepo) HeadObject(_ context.Context, _ string) (*storage.ObjectInfo, error) {
	if f.headErr != nil {
		return nil, f.headErr
	}
	if f.headInfo != nil {
		return f.headInfo, nil
	}
	return &storage.ObjectInfo{}, nil
}

func uploadServer(storageRepo *fakeStorageRepo) *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	return New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin, MediaPublicBaseURL: "https://cdn.example.com/media"},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{},
		&fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, storageRepo, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
}

func uploadURL(srv *http.Server, body string, withCookie, withOrigin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/upload-url", bytes.NewBufferString(body))
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

func TestUploadURLValidImage(t *testing.T) {
	rec := uploadURL(uploadServer(&fakeStorageRepo{}), `{"type":"image","mimeType":"image/jpeg","sizeBytes":1048576}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp uploadURLResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.UploadURL == "" || resp.StorageKey == "" || resp.PublicURL == "" || resp.ExpiresAt == "" {
		t.Fatalf("incomplete response: %+v", resp)
	}
	if !strings.HasSuffix(resp.StorageKey, ".jpg") {
		t.Fatalf("expected .jpg extension, got %q", resp.StorageKey)
	}
	if resp.PublicURL != "https://cdn.example.com/media/"+resp.StorageKey {
		t.Fatalf("publicUrl not built from base: %q", resp.PublicURL)
	}
}

func TestUploadURLValidVideo(t *testing.T) {
	rec := uploadURL(uploadServer(&fakeStorageRepo{}), `{"type":"video","mimeType":"video/mp4","sizeBytes":10485760}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp uploadURLResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if !strings.HasSuffix(resp.StorageKey, ".mp4") {
		t.Fatalf("expected .mp4 extension, got %q", resp.StorageKey)
	}
}

func TestUploadURLBadMime(t *testing.T) {
	rec := uploadURL(uploadServer(&fakeStorageRepo{}), `{"type":"image","mimeType":"image/gif","sizeBytes":1000}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestUploadURLSizeOverLimit(t *testing.T) {
	// 16MB image > 15MB limit
	rec := uploadURL(uploadServer(&fakeStorageRepo{}), `{"type":"image","mimeType":"image/png","sizeBytes":16777216}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestUploadURLZeroSize(t *testing.T) {
	rec := uploadURL(uploadServer(&fakeStorageRepo{}), `{"type":"image","mimeType":"image/png","sizeBytes":0}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestUploadURLUnauthenticated(t *testing.T) {
	rec := uploadURL(uploadServer(&fakeStorageRepo{}), `{"type":"image","mimeType":"image/png","sizeBytes":1000}`, false, true)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestUploadURLMissingOrigin(t *testing.T) {
	rec := uploadURL(uploadServer(&fakeStorageRepo{}), `{"type":"image","mimeType":"image/png","sizeBytes":1000}`, true, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestUploadURLKeyContainsUserID(t *testing.T) {
	sr := &fakeStorageRepo{}
	rec := uploadURL(uploadServer(sr), `{"type":"image","mimeType":"image/webp","sizeBytes":1000}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.HasPrefix(sr.lastKey, "users/me-id/uploads/") {
		t.Fatalf("key must be under the auth user's prefix, got %q", sr.lastKey)
	}
	var resp uploadURLResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if !strings.HasPrefix(resp.StorageKey, "users/me-id/uploads/") {
		t.Fatalf("response key not under user prefix: %q", resp.StorageKey)
	}
}

func TestUploadURLStorageError(t *testing.T) {
	rec := uploadURL(uploadServer(&fakeStorageRepo{createErr: errForTest}), `{"type":"image","mimeType":"image/png","sizeBytes":1000}`, true, true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
