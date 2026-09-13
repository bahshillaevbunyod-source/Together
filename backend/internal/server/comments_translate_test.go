package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"together/backend/internal/comment"
	"together/backend/internal/config"
	"together/backend/internal/post"
	"together/backend/internal/translation"
	"together/backend/internal/user"
)

// commentTranslateServer builds a white-box server where the authed viewer is
// `me` (with prefs) and can see a public post by author-id, using translator tr.
func commentTranslateServer(me *user.User, comments *fakeCommentRepo, tr translation.Service) *Server {
	author := mkUser("author-id", "author_user")
	users := &fakeUserRepo{byID: map[string]*user.User{me.ID: me, "author-id": author}}
	s := newServer(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users, activeSession(me.ID), &fakeFollowRepo{is: true}, &fakeBlockRepo{},
		&fakePostRepo{getPost: mkPost(post.VisibilityPublic, "author-id")}, &fakeLikeRepo{}, comments,
		&fakeMediaRepo{}, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{},
		&fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{},
		&fakeCommentNotifier{comments: comments}, &fakeConversationRepo{},
	)
	if tr != nil {
		s.translator = tr
	}
	return s
}

func viewerPrefs(enabled bool, preferred *string) *user.User {
	u := mkUser("me-id", "me_user")
	u.AutoTranslateEnabled = enabled
	u.PreferredLanguage = preferred
	return u
}

func createCommentResp(t *testing.T, s *Server) commentResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts/"+validPostID+"/comments", bytes.NewBufferString(`{"content":"nice"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	req.Header.Set("Origin", testOrigin)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp commentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	return resp
}

func TestCreateCommentTranslateDisabled(t *testing.T) {
	es := "es"
	s := commentTranslateServer(viewerPrefs(false, &es), &fakeCommentRepo{}, &fakeTranslator{})
	resp := createCommentResp(t, s)
	if resp.TranslatedContent != nil {
		t.Fatal("disabled: expected no translation")
	}
	if resp.Content != "nice" {
		t.Fatalf("original not preserved: %q", resp.Content)
	}
}

func TestCreateCommentTranslated(t *testing.T) {
	es := "es"
	s := commentTranslateServer(viewerPrefs(true, &es), &fakeCommentRepo{}, &fakeTranslator{})
	resp := createCommentResp(t, s)
	if resp.Content != "nice" {
		t.Fatalf("original not preserved: %q", resp.Content)
	}
	if resp.TranslatedContent == nil || *resp.TranslatedContent != "[es] nice" {
		t.Fatalf("unexpected translation: %v", resp.TranslatedContent)
	}
	if resp.TargetLanguage == nil || *resp.TargetLanguage != "es" {
		t.Fatalf("target missing: %v", resp.TargetLanguage)
	}
	if resp.SourceLanguage == nil || *resp.SourceLanguage != "auto" {
		t.Fatalf("source missing: %v", resp.SourceLanguage)
	}
}

func TestCreateCommentTranslateNoPreferredLanguage(t *testing.T) {
	s := commentTranslateServer(viewerPrefs(true, nil), &fakeCommentRepo{}, &fakeTranslator{})
	if createCommentResp(t, s).TranslatedContent != nil {
		t.Fatal("no preferred language: expected no translation")
	}
}

func TestCreateCommentTranslationErrorFallback(t *testing.T) {
	es := "es"
	s := commentTranslateServer(viewerPrefs(true, &es), &fakeCommentRepo{}, &fakeTranslator{err: errForTest})
	resp := createCommentResp(t, s)
	if resp.TranslatedContent != nil {
		t.Fatal("translation error: fields must stay nil")
	}
	if resp.Content != "nice" {
		t.Fatalf("original must survive translation error: %q", resp.Content)
	}
}

func TestListCommentsTranslatedIndependently(t *testing.T) {
	es := "es"
	c1 := mkCommentItem("11111111-1111-1111-1111-111111111111")
	c1.Content = "hello"
	c2 := mkCommentItem("22222222-2222-2222-2222-222222222222")
	c2.Content = "world"
	comments := &fakeCommentRepo{list: []comment.ListItem{c1, c2}}
	s := commentTranslateServer(viewerPrefs(true, &es), comments, &fakeTranslator{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+validPostID+"/comments", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp commentListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.Items[0].TranslatedContent == nil || *resp.Items[0].TranslatedContent != "[es] hello" {
		t.Fatalf("item0 translation wrong: %v", resp.Items[0].TranslatedContent)
	}
	if resp.Items[1].TranslatedContent == nil || *resp.Items[1].TranslatedContent != "[es] world" {
		t.Fatalf("item1 translation wrong: %v", resp.Items[1].TranslatedContent)
	}
	if resp.Items[0].Content != "hello" {
		t.Fatalf("item0 original not preserved: %q", resp.Items[0].Content)
	}
}

func TestListCommentsTranslateDisabled(t *testing.T) {
	es := "es"
	c1 := mkCommentItem("11111111-1111-1111-1111-111111111111")
	c1.Content = "hello"
	comments := &fakeCommentRepo{list: []comment.ListItem{c1}}
	s := commentTranslateServer(viewerPrefs(false, &es), comments, &fakeTranslator{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+validPostID+"/comments", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	var resp commentListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 1 || resp.Items[0].TranslatedContent != nil {
		t.Fatalf("disabled: expected no translation, got %+v", resp.Items)
	}
}
