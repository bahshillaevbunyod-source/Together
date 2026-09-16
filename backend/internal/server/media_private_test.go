package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"together/backend/internal/config"
	"together/backend/internal/media"
	"together/backend/internal/post"
	"together/backend/internal/storage"
	"together/backend/internal/user"
)

// ---- upload bucket routing --------------------------------------------------

func decodeUpload(t *testing.T, rec *httptest.ResponseRecorder) uploadURLResponse {
	t.Helper()
	var resp uploadURLResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v (%s)", err, rec.Body.String())
	}
	return resp
}

func TestUploadRestrictedUsesPrivateBucket(t *testing.T) {
	sr := &fakeStorageRepo{}
	rec := uploadURL(uploadServer(sr), `{"type":"image","mimeType":"image/webp","sizeBytes":1024,"visibility":"followers"}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if sr.lastCreateClass != storage.ClassPrivate {
		t.Fatalf("restricted upload must target the private bucket, got %q", sr.lastCreateClass)
	}
	resp := decodeUpload(t, rec)
	if !strings.HasPrefix(resp.StorageKey, "users/me-id/private/") {
		t.Fatalf("restricted upload must use the private namespace, got %q", resp.StorageKey)
	}
	if resp.PublicURL != "" {
		t.Fatalf("restricted upload must not expose a public URL, got %q", resp.PublicURL)
	}
}

func TestUploadPublicUsesPublicBucket(t *testing.T) {
	sr := &fakeStorageRepo{}
	rec := uploadURL(uploadServer(sr), `{"type":"image","mimeType":"image/webp","sizeBytes":1024,"visibility":"public"}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if sr.lastCreateClass != storage.ClassPublic {
		t.Fatalf("public upload must target the public bucket, got %q", sr.lastCreateClass)
	}
	resp := decodeUpload(t, rec)
	if !strings.HasPrefix(resp.StorageKey, "users/me-id/uploads/") {
		t.Fatalf("public upload must use the uploads namespace, got %q", resp.StorageKey)
	}
	if resp.PublicURL == "" {
		t.Fatalf("public upload should expose a public URL")
	}
}

func TestUploadAvatarStaysPublicIgnoringVisibility(t *testing.T) {
	sr := &fakeStorageRepo{}
	// Even with a restricted visibility hint, avatars are always public.
	rec := uploadURL(uploadServer(sr), `{"type":"image","mimeType":"image/png","sizeBytes":1024,"purpose":"avatar","visibility":"private"}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if sr.lastCreateClass != storage.ClassPublic {
		t.Fatalf("avatar must stay in the public bucket, got %q", sr.lastCreateClass)
	}
	if !strings.HasPrefix(decodeUpload(t, rec).StorageKey, "users/me-id/avatars/") {
		t.Fatalf("avatar must use the avatars namespace")
	}
}

func TestUploadInvalidVisibility(t *testing.T) {
	rec := uploadURL(uploadServer(&fakeStorageRepo{}), `{"type":"image","mimeType":"image/png","sizeBytes":1024,"visibility":"bogus"}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid visibility, got %d", rec.Code)
	}
}

func TestUploadPrivateNotConfigured(t *testing.T) {
	sr := &fakeStorageRepo{privateOff: true}
	rec := uploadURL(uploadServer(sr), `{"type":"image","mimeType":"image/png","sizeBytes":1024,"visibility":"private"}`, true, true)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when private storage unavailable, got %d", rec.Code)
	}
}

// ---- create-post storage-class enforcement ---------------------------------

func TestCreateRestrictedRejectsPublicMedia(t *testing.T) {
	rec := createPost(createServer(&fakePostCreate{}, okStorage()),
		`{"visibility":"followers","storageKeys":["users/me-id/uploads/x.png"]}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("restricted post must reject public-bucket media, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestCreatePublicRejectsPrivateMedia(t *testing.T) {
	rec := createPost(createServer(&fakePostCreate{}, okStorage()),
		`{"visibility":"public","storageKeys":["users/me-id/private/x.png"]}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("public post must reject private-bucket media, got %d", rec.Code)
	}
}

func TestCreateRestrictedAcceptsPrivateMedia(t *testing.T) {
	pc := &fakePostCreate{}
	rec := createPost(createServer(pc, okStorage()),
		`{"visibility":"private","storageKeys":["users/me-id/private/x.png"]}`, true, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	if len(pc.lastMedia) != 1 || pc.lastMedia[0].StorageKey != "users/me-id/private/x.png" {
		t.Fatalf("private media not attached: %+v", pc.lastMedia)
	}
}

func TestCreateForeignMediaRejected(t *testing.T) {
	rec := createPost(createServer(&fakePostCreate{}, okStorage()),
		`{"visibility":"public","storageKeys":["users/other-user/uploads/x.png"]}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("another user's media must be rejected, got %d", rec.Code)
	}
}

// ---- read authorization + presigned delivery -------------------------------

func privateReadServer(sessions *fakeSessionRepo, follows *fakeFollowRepo, blocks *fakeBlockRepo, posts *fakePostRepo, mediaRepo *fakeMediaRepo, sr *fakeStorageRepo) *http.Server {
	users := &fakeUserRepo{byID: map[string]*user.User{
		"me-id":     mkUser("me-id", "me_user"),
		"author-id": mkUser("author-id", "author_user"),
	}}
	return New(
		config.Config{Env: "test", Port: "8080", MediaPublicBaseURL: "https://cdn.example.com/media"},
		fakePinger{}, users, sessions, follows, blocks, posts, &fakeLikeRepo{}, &fakeCommentRepo{}, mediaRepo, sr, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
}

type readMediaResp struct {
	Media []struct {
		URL string `json:"url"`
	} `json:"media"`
}

func decodeReadMedia(t *testing.T, rec *httptest.ResponseRecorder) readMediaResp {
	t.Helper()
	var resp readMediaResp
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v (%s)", err, rec.Body.String())
	}
	return resp
}

func privateMedia(author string) *fakeMediaRepo {
	return &fakeMediaRepo{byPost: []media.Media{
		{ID: "m1", PostID: validPostID, Type: media.TypeImage, StorageKey: "users/" + author + "/private/x.webp", MimeType: "image/webp", SortOrder: 0},
	}}
}

func TestPrivatePostOwnerGetsSignedURL(t *testing.T) {
	sr := &fakeStorageRepo{}
	posts := &fakePostRepo{getPost: mkPost(post.VisibilityPrivate, "me-id")}
	srv := privateReadServer(activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{}, posts, privateMedia("me-id"), sr)
	rec := getPost(srv, validPostID, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("owner should read own private post, got %d", rec.Code)
	}
	m := decodeReadMedia(t, rec)
	if len(m.Media) != 1 || !strings.HasPrefix(m.Media[0].URL, "https://signed.example.com/private/") {
		t.Fatalf("owner should get a signed private URL, got %+v", m.Media)
	}
	if sr.lastPresignClass != storage.ClassPrivate {
		t.Fatalf("expected a private presign, got %q", sr.lastPresignClass)
	}
}

func TestPrivatePostNonOwnerNoSignedURL(t *testing.T) {
	sr := &fakeStorageRepo{}
	posts := &fakePostRepo{getPost: mkPost(post.VisibilityPrivate, "author-id")}
	srv := privateReadServer(activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{}, posts, privateMedia("author-id"), sr)
	rec := getPost(srv, validPostID, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("non-owner must not read a private post, got %d", rec.Code)
	}
	if sr.lastPresignClass != "" {
		t.Fatalf("no signed URL may be generated for an unauthorized viewer, got %q", sr.lastPresignClass)
	}
}

func TestFollowersPostFollowerGetsSignedURL(t *testing.T) {
	sr := &fakeStorageRepo{}
	posts := &fakePostRepo{getPost: mkPost(post.VisibilityFollowers, "author-id")}
	srv := privateReadServer(activeSession("me-id"), &fakeFollowRepo{is: true}, &fakeBlockRepo{}, posts, privateMedia("author-id"), sr)
	rec := getPost(srv, validPostID, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("follower should read a followers-only post, got %d", rec.Code)
	}
	m := decodeReadMedia(t, rec)
	if len(m.Media) != 1 || !strings.HasPrefix(m.Media[0].URL, "https://signed.example.com/private/") {
		t.Fatalf("follower should get a signed private URL, got %+v", m.Media)
	}
}

func TestFollowersPostNonFollowerDenied(t *testing.T) {
	sr := &fakeStorageRepo{}
	posts := &fakePostRepo{getPost: mkPost(post.VisibilityFollowers, "author-id")}
	srv := privateReadServer(activeSession("me-id"), &fakeFollowRepo{is: false}, &fakeBlockRepo{}, posts, privateMedia("author-id"), sr)
	rec := getPost(srv, validPostID, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("non-follower must be denied, got %d", rec.Code)
	}
	if sr.lastPresignClass != "" {
		t.Fatalf("no signed URL for a non-follower, got %q", sr.lastPresignClass)
	}
}

func TestBlockedViewerDenied(t *testing.T) {
	sr := &fakeStorageRepo{}
	posts := &fakePostRepo{getPost: mkPost(post.VisibilityFollowers, "author-id")}
	// Follows would otherwise grant access, but a block overrides it.
	srv := privateReadServer(activeSession("me-id"), &fakeFollowRepo{is: true}, &fakeBlockRepo{hasBetween: true}, posts, privateMedia("author-id"), sr)
	rec := getPost(srv, validPostID, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("blocked viewer must be denied, got %d", rec.Code)
	}
	if sr.lastPresignClass != "" {
		t.Fatalf("no signed URL for a blocked viewer, got %q", sr.lastPresignClass)
	}
}

func TestLegacyAndPublicMediaServedPublicly(t *testing.T) {
	sr := &fakeStorageRepo{}
	posts := &fakePostRepo{getPost: mkPost(post.VisibilityPublic, "author-id")}
	// Legacy/public media lives under uploads/ and must keep its permanent URL.
	mediaRepo := &fakeMediaRepo{byPost: []media.Media{
		{ID: "m1", PostID: validPostID, Type: media.TypeImage, StorageKey: "users/author-id/uploads/legacy.jpg", MimeType: "image/jpeg", SortOrder: 0},
	}}
	srv := privateReadServer(&fakeSessionRepo{}, &fakeFollowRepo{}, &fakeBlockRepo{}, posts, mediaRepo, sr)
	rec := getPost(srv, validPostID, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("public post should be readable, got %d", rec.Code)
	}
	m := decodeReadMedia(t, rec)
	if len(m.Media) != 1 || m.Media[0].URL != "https://cdn.example.com/media/users/author-id/uploads/legacy.jpg" {
		t.Fatalf("public/legacy media must keep its permanent public URL, got %+v", m.Media)
	}
	if sr.lastPresignClass != "" {
		t.Fatalf("public media must not be presigned, got %q", sr.lastPresignClass)
	}
}

// ---- deletion targets the correct bucket -----------------------------------

func TestDeletePrivatePostMediaUsesPrivateBucket(t *testing.T) {
	mediaRepo := &fakeMediaRepo{byPost: []media.Media{
		mkPostMedia("users/me-id/private/a.webp"),
	}}
	sr := &fakeStorageRepo{}
	rec := deletePost(editDeletePostServerFull(&fakePostRepo{getPost: ownPost()}, mediaRepo, sr), true, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if len(sr.deleted) != 1 || sr.deleted[0] != "users/me-id/private/a.webp" {
		t.Fatalf("private object not deleted: %v", sr.deleted)
	}
	if len(sr.deletedClasses) != 1 || sr.deletedClasses[0] != storage.ClassPrivate {
		t.Fatalf("private media must be deleted from the private bucket, got %v", sr.deletedClasses)
	}
}
