package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"together/backend/internal/config"
	"together/backend/internal/post"
	"together/backend/internal/translation"
	"together/backend/internal/user"
)

// postTranslateServer builds a white-box server where the authed viewer is `me`
// (with prefs) and can see a public post by author-id, using translator `tr`.
func postTranslateServer(me *user.User, posts *fakePostRepo, tr translation.Service) *Server {
	author := mkUser("author-id", "author_user")
	users := &fakeUserRepo{byID: map[string]*user.User{me.ID: me, "author-id": author}}
	s := newServer(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users, activeSession(me.ID), &fakeFollowRepo{is: true}, &fakeBlockRepo{},
		posts, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{},
		&fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{},
		&fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
	if tr != nil {
		s.translator = tr
	}
	return s
}

func viewerWithTranslate(enabled bool, preferred *string) *user.User {
	u := mkUser("me-id", "me_user")
	u.AutoTranslateEnabled = enabled
	u.PreferredLanguage = preferred
	return u
}

func getPostResp(t *testing.T, s *Server) postResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+validPostID, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp postResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	return resp
}

func publicPostRepo() *fakePostRepo {
	return &fakePostRepo{getPost: mkPost(post.VisibilityPublic, "author-id")}
}

func TestGetPostTranslateDisabled(t *testing.T) {
	es := "es"
	s := postTranslateServer(viewerWithTranslate(false, &es), publicPostRepo(), &fakeTranslator{})
	resp := getPostResp(t, s)
	if resp.TranslatedContent != nil {
		t.Fatal("disabled: expected no translation")
	}
	if resp.Content == nil || *resp.Content != "hi" {
		t.Fatalf("original content not preserved: %v", resp.Content)
	}
}

func TestGetPostTranslated(t *testing.T) {
	es := "es"
	s := postTranslateServer(viewerWithTranslate(true, &es), publicPostRepo(), &fakeTranslator{})
	resp := getPostResp(t, s)
	if resp.Content == nil || *resp.Content != "hi" {
		t.Fatalf("original not preserved: %v", resp.Content)
	}
	if resp.TranslatedContent == nil || *resp.TranslatedContent != "[es] hi" {
		t.Fatalf("unexpected translation: %v", resp.TranslatedContent)
	}
	if resp.TargetLanguage == nil || *resp.TargetLanguage != "es" {
		t.Fatalf("target missing: %v", resp.TargetLanguage)
	}
	if resp.SourceLanguage == nil || *resp.SourceLanguage != "auto" {
		t.Fatalf("source missing: %v", resp.SourceLanguage)
	}
}

func TestGetPostTranslateNoPreferredLanguage(t *testing.T) {
	s := postTranslateServer(viewerWithTranslate(true, nil), publicPostRepo(), &fakeTranslator{})
	if getPostResp(t, s).TranslatedContent != nil {
		t.Fatal("no preferred language: expected no translation")
	}
}

func TestGetPostTranslationErrorFallback(t *testing.T) {
	es := "es"
	s := postTranslateServer(viewerWithTranslate(true, &es), publicPostRepo(), &fakeTranslator{err: errForTest})
	resp := getPostResp(t, s)
	if resp.TranslatedContent != nil {
		t.Fatal("translation error: fields must stay nil")
	}
	if resp.Content == nil || *resp.Content != "hi" {
		t.Fatalf("original must survive translation error: %v", resp.Content)
	}
}

func TestFeedItemsTranslatedIndependently(t *testing.T) {
	es := "es"
	item1 := mkFeedItem("11111111-1111-1111-1111-111111111111")
	c1 := "hello"
	item1.Content = &c1
	item2 := mkFeedItem("22222222-2222-2222-2222-222222222222")
	c2 := "world"
	item2.Content = &c2
	item3 := mkFeedItem("33333333-3333-3333-3333-333333333333")
	item3.Content = nil // media-only: nothing to translate

	posts := &fakePostRepo{feed: []post.FeedItem{item1, item2, item3}}
	s := postTranslateServer(viewerWithTranslate(true, &es), posts, &fakeTranslator{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/feed", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp feedResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(resp.Items))
	}
	if resp.Items[0].TranslatedContent == nil || *resp.Items[0].TranslatedContent != "[es] hello" {
		t.Fatalf("item0 translation wrong: %v", resp.Items[0].TranslatedContent)
	}
	if resp.Items[1].TranslatedContent == nil || *resp.Items[1].TranslatedContent != "[es] world" {
		t.Fatalf("item1 translation wrong: %v", resp.Items[1].TranslatedContent)
	}
	// media-only item has no text -> no translation
	if resp.Items[2].TranslatedContent != nil {
		t.Fatalf("item2 (no text) should not be translated: %v", resp.Items[2].TranslatedContent)
	}
	// originals preserved
	if resp.Items[0].Content == nil || *resp.Items[0].Content != "hello" {
		t.Fatalf("item0 original not preserved: %v", resp.Items[0].Content)
	}
}

func TestFeedTranslateDisabled(t *testing.T) {
	es := "es"
	item := mkFeedItem("11111111-1111-1111-1111-111111111111")
	c := "hello"
	item.Content = &c
	posts := &fakePostRepo{feed: []post.FeedItem{item}}
	s := postTranslateServer(viewerWithTranslate(false, &es), posts, &fakeTranslator{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/feed", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	var resp feedResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 1 || resp.Items[0].TranslatedContent != nil {
		t.Fatalf("disabled: expected no translation, got %+v", resp.Items)
	}
}
