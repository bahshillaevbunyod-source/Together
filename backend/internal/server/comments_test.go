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

	"together/backend/internal/comment"
	"together/backend/internal/config"
	"together/backend/internal/post"
	"together/backend/internal/user"
)

// fakeCommentRepo is a test double for comment.Repository.
type fakeCommentRepo struct {
	createErr  error
	listErr    error
	countErr   error
	last       comment.CreateInput
	list       []comment.ListItem
	count      int64
	getComment *comment.Comment
	getErr     error
	updateErr  error
	deleteErr  error
	deleted    []string
}

func (f *fakeCommentRepo) GetByID(_ context.Context, _ string) (*comment.Comment, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.getComment == nil {
		return nil, comment.ErrNotFound
	}
	return f.getComment, nil
}

func (f *fakeCommentRepo) UpdateContent(_ context.Context, id, content string) (*comment.Comment, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	base := f.getComment
	now := time.Now()
	return &comment.Comment{
		ID: id, PostID: base.PostID, AuthorID: base.AuthorID,
		Content: content, CreatedAt: base.CreatedAt, UpdatedAt: now,
	}, nil
}

func (f *fakeCommentRepo) Delete(_ context.Context, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *fakeCommentRepo) Create(_ context.Context, in comment.CreateInput) (*comment.Comment, error) {
	f.last = in
	if f.createErr != nil {
		return nil, f.createErr
	}
	now := time.Now()
	return &comment.Comment{
		ID: "comment-1", PostID: in.PostID, AuthorID: in.AuthorID,
		Content: in.Content, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (f *fakeCommentRepo) ListByPost(_ context.Context, _ string, _ *comment.Cursor, limit int) ([]comment.ListItem, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if len(f.list) > limit {
		return f.list[:limit], nil
	}
	return f.list, nil
}

func (f *fakeCommentRepo) CountByPost(_ context.Context, _ string) (int64, error) {
	return f.count, f.countErr
}

// fakeCommentNotifier is a test double for the atomic comment+notify service.
// It delegates to an optional comment repo so creation results and errors still
// surface, and records the inputs (including the post author) it was given.
type fakeCommentNotifier struct {
	comments comment.Repository
	err      error
	calls    []commentCall
}

type commentCall struct {
	in           comment.CreateInput
	postAuthorID string
}

func (f *fakeCommentNotifier) CreateComment(ctx context.Context, in comment.CreateInput, postAuthorID string) (*comment.Comment, error) {
	f.calls = append(f.calls, commentCall{in: in, postAuthorID: postAuthorID})
	if f.err != nil {
		return nil, f.err
	}
	if f.comments != nil {
		return f.comments.Create(ctx, in)
	}
	return &comment.Comment{ID: "comment-1", PostID: in.PostID, AuthorID: in.AuthorID, Content: in.Content}, nil
}

// commentServer: me can see a public post; the given comment repo is used.
func commentServer(comments *fakeCommentRepo, p *post.Post) *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me, "author-id": mkUser("author-id", "author_user")}}
	return New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{is: true}, &fakeBlockRepo{},
		&fakePostRepo{getPost: p}, &fakeLikeRepo{}, comments, &fakeMediaRepo{}, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{comments: comments}, &fakeConversationRepo{},
	)
}

func createComment(srv *http.Server, body string, withCookie, withOrigin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts/"+validPostID+"/comments", bytes.NewBufferString(body))
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

func listComments(srv *http.Server, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+validPostID+"/comments"+query, nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func publicPost() *post.Post { return mkPost(post.VisibilityPublic, "author-id") }

func TestCreateCommentSuccess(t *testing.T) {
	comments := &fakeCommentRepo{}
	rec := createComment(commentServer(comments, publicPost()), `{"content":"nice post"}`, true, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	if comments.last.AuthorID != "me-id" || comments.last.PostID != validPostID {
		t.Fatalf("comment input wrong: %+v", comments.last)
	}
}

func TestCreateCommentEmpty(t *testing.T) {
	rec := createComment(commentServer(&fakeCommentRepo{}, publicPost()), `{"content":"   "}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreateCommentTooLong(t *testing.T) {
	long := make([]byte, 2001)
	for i := range long {
		long[i] = 'a'
	}
	rec := createComment(commentServer(&fakeCommentRepo{}, publicPost()), `{"content":"`+string(long)+`"}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreateCommentInaccessiblePost(t *testing.T) {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	srv := New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{is: false}, &fakeBlockRepo{},
		&fakePostRepo{getPost: mkPost(post.VisibilityPrivate, "author-id")}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
	rec := createComment(srv, `{"content":"hi"}`, true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestCreateCommentUnauthenticated(t *testing.T) {
	rec := createComment(commentServer(&fakeCommentRepo{}, publicPost()), `{"content":"hi"}`, false, true)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCreateCommentMissingOrigin(t *testing.T) {
	rec := createComment(commentServer(&fakeCommentRepo{}, publicPost()), `{"content":"hi"}`, true, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestCreateCommentRepositoryError(t *testing.T) {
	rec := createComment(commentServer(&fakeCommentRepo{createErr: errForTest}, publicPost()), `{"content":"hi"}`, true, true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func mkCommentItem(id string) comment.ListItem {
	now := time.Now()
	return comment.ListItem{
		ID: id, AuthorID: "author-id", Content: "hi", CreatedAt: now, UpdatedAt: now,
		AuthorUsername: "author_user", AuthorDisplayName: "Author",
	}
}

func TestListCommentsSuccess(t *testing.T) {
	comments := &fakeCommentRepo{list: []comment.ListItem{mkCommentItem("11111111-1111-1111-1111-111111111111")}}
	rec := listComments(commentServer(comments, publicPost()), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp commentListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
}

func TestListCommentsPagination(t *testing.T) {
	comments := &fakeCommentRepo{list: []comment.ListItem{
		mkCommentItem("11111111-1111-1111-1111-111111111111"),
		mkCommentItem("22222222-2222-2222-2222-222222222222"),
		mkCommentItem("33333333-3333-3333-3333-333333333333"),
	}}
	rec := listComments(commentServer(comments, publicPost()), "?limit=2")
	var resp commentListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 2 || resp.NextCursor == "" {
		t.Fatalf("pagination failed: %d items, cursor %q", len(resp.Items), resp.NextCursor)
	}
	if _, ok := parseCommentCursor(resp.NextCursor); !ok {
		t.Fatal("nextCursor not decodable")
	}
}

func TestListCommentsInvalidCursor(t *testing.T) {
	rec := listComments(commentServer(&fakeCommentRepo{}, publicPost()), "?cursor=@@@")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestListCommentsRepositoryError(t *testing.T) {
	rec := listComments(commentServer(&fakeCommentRepo{listErr: errForTest}, publicPost()), "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestGetPostIncludesCommentsCount(t *testing.T) {
	me := mkUser("me-id", "me_user")
	author := mkUser("author-id", "author_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me, "author-id": author}}
	srv := New(
		config.Config{Env: "test", Port: "8080"},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{},
		&fakePostRepo{getPost: publicPost()}, &fakeLikeRepo{}, &fakeCommentRepo{count: 4}, &fakeMediaRepo{}, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
	rec := getPost(srv, validPostID, true)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if m["commentsCount"].(float64) != 4 {
		t.Fatalf("post response missing commentsCount: %v", m)
	}
}

func TestFeedIncludesCommentsCount(t *testing.T) {
	item := mkFeedItem("11111111-1111-1111-1111-111111111111")
	item.CommentsCount = 9
	rec := getFeed(feedServer(&fakePostRepo{feed: []post.FeedItem{item}}), "")
	var resp feedResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 1 || resp.Items[0].CommentsCount != 9 {
		t.Fatalf("feed item missing commentsCount: %+v", resp.Items)
	}
}

// ---- edit / delete ----

const commentID = "cccccccc-1111-2222-3333-444444444444"

func ownComment() *comment.Comment {
	now := time.Now()
	return &comment.Comment{ID: commentID, PostID: validPostID, AuthorID: "me-id", Content: "old", CreatedAt: now, UpdatedAt: now}
}

func editDeleteServer(comments *fakeCommentRepo) *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	return New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{},
		&fakePostRepo{}, &fakeLikeRepo{}, comments, &fakeMediaRepo{}, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
}

func editComment(srv *http.Server, body string, withCookie, withOrigin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/comments/"+commentID, bytes.NewBufferString(body))
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

func deleteComment(srv *http.Server, withCookie, withOrigin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/comments/"+commentID, nil)
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

func TestEditOwnComment(t *testing.T) {
	rec := editComment(editDeleteServer(&fakeCommentRepo{getComment: ownComment()}), `{"content":"updated"}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if m["content"] != "updated" {
		t.Fatalf("content not updated: %v", m)
	}
	for _, leak := range []string{"email", "phone", "password"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Fatalf("response leaked %q: %s", leak, rec.Body.String())
		}
	}
}

func TestEditCommentEmpty(t *testing.T) {
	rec := editComment(editDeleteServer(&fakeCommentRepo{getComment: ownComment()}), `{"content":"  "}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestEditCommentTooLong(t *testing.T) {
	rec := editComment(editDeleteServer(&fakeCommentRepo{getComment: ownComment()}), `{"content":"`+strings.Repeat("a", 2001)+`"}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestEditCommentExactly2000Unicode(t *testing.T) {
	rec := editComment(editDeleteServer(&fakeCommentRepo{getComment: ownComment()}), `{"content":"`+strings.Repeat("é", 2000)+`"}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestEditOthersComment404(t *testing.T) {
	c := ownComment()
	c.AuthorID = "someone-else"
	rec := editComment(editDeleteServer(&fakeCommentRepo{getComment: c}), `{"content":"x"}`, true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDeleteOwnComment(t *testing.T) {
	comments := &fakeCommentRepo{getComment: ownComment()}
	rec := deleteComment(editDeleteServer(comments), true, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("204 must have empty body, got %q", rec.Body.String())
	}
	if len(comments.deleted) != 1 || comments.deleted[0] != commentID {
		t.Fatalf("comment not deleted: %v", comments.deleted)
	}
}

func TestDeleteOthersComment404(t *testing.T) {
	c := ownComment()
	c.AuthorID = "someone-else"
	rec := deleteComment(editDeleteServer(&fakeCommentRepo{getComment: c}), true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDeleteUnknownComment404(t *testing.T) {
	rec := deleteComment(editDeleteServer(&fakeCommentRepo{getComment: nil}), true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestEditCommentUnauthenticated(t *testing.T) {
	rec := editComment(editDeleteServer(&fakeCommentRepo{getComment: ownComment()}), `{"content":"x"}`, false, true)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestEditCommentMissingOrigin(t *testing.T) {
	rec := editComment(editDeleteServer(&fakeCommentRepo{getComment: ownComment()}), `{"content":"x"}`, true, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestEditCommentRepositoryError(t *testing.T) {
	rec := editComment(editDeleteServer(&fakeCommentRepo{getComment: ownComment(), updateErr: errForTest}), `{"content":"x"}`, true, true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
