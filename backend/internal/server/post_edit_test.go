package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"together/backend/internal/config"
	"together/backend/internal/media"
	"together/backend/internal/post"
	"together/backend/internal/user"
)

func ownPost() *post.Post { return mkPost(post.VisibilityPublic, "me-id") }

func editDeletePostServer(posts *fakePostRepo) *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	return New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{},
		posts, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
}

func editPost(srv *http.Server, body string, withCookie, withOrigin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/posts/"+validPostID, bytes.NewBufferString(body))
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

func deletePost(srv *http.Server, withCookie, withOrigin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/posts/"+validPostID, nil)
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

func TestEditPostContent(t *testing.T) {
	rec := editPost(editDeletePostServer(&fakePostRepo{getPost: ownPost()}), `{"content":"updated"}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	for _, leak := range []string{"email", "phone", "password"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Fatalf("response leaked %q: %s", leak, rec.Body.String())
		}
	}
}

func TestEditPostVisibility(t *testing.T) {
	rec := editPost(editDeletePostServer(&fakePostRepo{getPost: ownPost()}), `{"visibility":"private"}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestEditPostBothFields(t *testing.T) {
	rec := editPost(editDeletePostServer(&fakePostRepo{getPost: ownPost()}), `{"content":"x","visibility":"followers"}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestEditPostEmptyPatch(t *testing.T) {
	rec := editPost(editDeletePostServer(&fakePostRepo{getPost: ownPost()}), `{}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestEditPostEmptyContent(t *testing.T) {
	rec := editPost(editDeletePostServer(&fakePostRepo{getPost: ownPost()}), `{"content":"   "}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestEditPostTooLong(t *testing.T) {
	rec := editPost(editDeletePostServer(&fakePostRepo{getPost: ownPost()}), `{"content":"`+strings.Repeat("a", 5001)+`"}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestEditPostExactly5000Unicode(t *testing.T) {
	rec := editPost(editDeletePostServer(&fakePostRepo{getPost: ownPost()}), `{"content":"`+strings.Repeat("é", 5000)+`"}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestEditPostInvalidVisibility(t *testing.T) {
	rec := editPost(editDeletePostServer(&fakePostRepo{getPost: ownPost()}), `{"visibility":"secret"}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestEditOthersPost404(t *testing.T) {
	p := ownPost()
	p.AuthorID = "someone-else"
	rec := editPost(editDeletePostServer(&fakePostRepo{getPost: p}), `{"content":"x"}`, true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDeleteOwnPost(t *testing.T) {
	posts := &fakePostRepo{getPost: ownPost()}
	rec := deletePost(editDeletePostServer(posts), true, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("204 must have empty body, got %q", rec.Body.String())
	}
	if len(posts.deleted) != 1 || posts.deleted[0] != validPostID {
		t.Fatalf("post not deleted: %v", posts.deleted)
	}
}

// editDeletePostServerFull injects a media repo and storage double so the
// deleted-post media cleanup can be observed.
func editDeletePostServerFull(posts *fakePostRepo, mediaRepo *fakeMediaRepo, sr *fakeStorageRepo) *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	return New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin, MediaPublicBaseURL: "https://cdn.example.com/media"},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{},
		posts, &fakeLikeRepo{}, &fakeCommentRepo{}, mediaRepo, sr, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
}

func mkPostMedia(key string) media.Media {
	return media.Media{ID: "m-" + key, PostID: validPostID, Type: media.TypeImage, StorageKey: key, MimeType: "image/webp"}
}

func TestDeletePostRemovesOwnMedia(t *testing.T) {
	mediaRepo := &fakeMediaRepo{byPost: []media.Media{
		mkPostMedia("users/me-id/uploads/a.webp"),
		mkPostMedia("users/me-id/uploads/b.webp"),
	}}
	sr := &fakeStorageRepo{}
	rec := deletePost(editDeletePostServerFull(&fakePostRepo{getPost: ownPost()}, mediaRepo, sr), true, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d (%s)", rec.Code, rec.Body.String())
	}
	if len(sr.deleted) != 2 {
		t.Fatalf("expected both owned objects deleted, got %v", sr.deleted)
	}
}

func TestDeletePostSkipsForeignAndAvatarKeys(t *testing.T) {
	mediaRepo := &fakeMediaRepo{byPost: []media.Media{
		mkPostMedia("users/other-user/uploads/x.webp"),    // another user's namespace
		mkPostMedia("users/me-id/avatars/y.webp"),         // avatar namespace (not uploads)
		mkPostMedia("users/me-id/uploads/../escape.webp"), // traversal-shaped
		mkPostMedia("users/me-id/uploads/ok.webp"),        // the only deletable one
	}}
	sr := &fakeStorageRepo{}
	deletePost(editDeletePostServerFull(&fakePostRepo{getPost: ownPost()}, mediaRepo, sr), true, true)
	if len(sr.deleted) != 1 || sr.deleted[0] != "users/me-id/uploads/ok.webp" {
		t.Fatalf("only the owner's own uploads object should be deleted, got %v", sr.deleted)
	}
}

func TestDeletePostStorageErrorStill204(t *testing.T) {
	mediaRepo := &fakeMediaRepo{byPost: []media.Media{mkPostMedia("users/me-id/uploads/a.webp")}}
	sr := &fakeStorageRepo{deleteErr: errForTest}
	rec := deletePost(editDeletePostServerFull(&fakePostRepo{getPost: ownPost()}, mediaRepo, sr), true, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("storage failure must not fail the delete, got %d", rec.Code)
	}
}

func TestDeletePostMediaListErrorStill204(t *testing.T) {
	mediaRepo := &fakeMediaRepo{listErr: errForTest}
	sr := &fakeStorageRepo{}
	posts := &fakePostRepo{getPost: ownPost()}
	rec := deletePost(editDeletePostServerFull(posts, mediaRepo, sr), true, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if len(posts.deleted) != 1 {
		t.Fatalf("post must still be deleted when media list fails, got %v", posts.deleted)
	}
	if len(sr.deleted) != 0 {
		t.Fatalf("no storage cleanup when media list failed, got %v", sr.deleted)
	}
}

func TestDeleteOthersPost404(t *testing.T) {
	p := ownPost()
	p.AuthorID = "someone-else"
	rec := deletePost(editDeletePostServer(&fakePostRepo{getPost: p}), true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDeleteUnknownPost404(t *testing.T) {
	rec := deletePost(editDeletePostServer(&fakePostRepo{getPost: nil}), true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestEditPostUnauthenticated(t *testing.T) {
	rec := editPost(editDeletePostServer(&fakePostRepo{getPost: ownPost()}), `{"content":"x"}`, false, true)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestEditPostMissingOrigin(t *testing.T) {
	rec := editPost(editDeletePostServer(&fakePostRepo{getPost: ownPost()}), `{"content":"x"}`, true, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestEditPostRepositoryError(t *testing.T) {
	rec := editPost(editDeletePostServer(&fakePostRepo{getPost: ownPost(), updateErr: errForTest}), `{"content":"x"}`, true, true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
