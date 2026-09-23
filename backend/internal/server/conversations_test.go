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
	"together/backend/internal/conversation"
	"together/backend/internal/realtime"
	"together/backend/internal/translation"
	"together/backend/internal/user"
)

// fakeConversationRepo is a test double for conversation.Repository.
type fakeConversationRepo struct {
	openConv           *conversation.Conversation
	openErr            error
	messages           []conversation.Message
	listErr            error
	lastCur            *conversation.MessageCursor
	lastLimit          int
	createErr          error
	recipientID        string
	lastContent        string
	lastSender         string
	message            *conversation.Message
	contextText        string
	contextErr         error
	metadataErr        error
	metadataCalls      int
	metadataLanguage   *string
	metadataConfidence *float64
	metadataResolution string
	convList           []conversation.ListItem
	convListErr        error
	lastConvCur        *conversation.ConversationCursor
	markFound          bool
	markErr            error
}

func (f *fakeConversationRepo) OpenPrivateConversation(_ context.Context, _, _ string) (*conversation.Conversation, error) {
	return f.openConv, f.openErr
}

func (f *fakeConversationRepo) CreateMessage(_ context.Context, conversationID, senderID, content string) (*conversation.Message, string, error) {
	f.lastContent = content
	f.lastSender = senderID
	if f.createErr != nil {
		return nil, "", f.createErr
	}
	recipient := f.recipientID
	if recipient == "" {
		recipient = "u2"
	}
	// Mirror the repository's trimming so the handler response reflects it.
	f.message = &conversation.Message{ID: "m-1", ConversationID: conversationID, SenderID: senderID, Content: strings.TrimSpace(content), CreatedAt: time.Now()}
	return f.message, recipient, nil
}

func (f *fakeConversationRepo) CreateMessageWithAttachment(_ context.Context, conversationID, senderID, content string, attachment conversation.Attachment) (*conversation.Message, string, error) {
	f.message = &conversation.Message{ID: "m-attachment", ConversationID: conversationID, SenderID: senderID, Content: strings.TrimSpace(content), CreatedAt: time.Now(), Attachment: &attachment}
	return f.message, "u2", nil
}

func (f *fakeConversationRepo) SetMessageLanguageMetadata(_ context.Context, _ string, _ string, language *string, confidence *float64, resolution string) error {
	f.metadataCalls++
	f.metadataLanguage = language
	f.metadataConfidence = confidence
	f.metadataResolution = resolution
	if f.metadataErr != nil {
		return f.metadataErr
	}
	return nil
}

func (f *fakeConversationRepo) RecentSenderContext(_ context.Context, _, _, _ string) (string, error) {
	if f.contextErr != nil {
		return "", f.contextErr
	}
	return f.contextText, nil
}

func (f *fakeConversationRepo) ListMessages(_ context.Context, _, _ string, cur *conversation.MessageCursor, limit int) ([]conversation.Message, error) {
	f.lastCur = cur
	f.lastLimit = limit
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.messages, nil
}

func (f *fakeConversationRepo) ListConversations(_ context.Context, _ string, cur *conversation.ConversationCursor, limit int) ([]conversation.ListItem, error) {
	f.lastConvCur = cur
	f.lastLimit = limit
	if f.convListErr != nil {
		return nil, f.convListErr
	}
	return f.convList, nil
}

func (f *fakeConversationRepo) MarkConversationRead(_ context.Context, _, _ string) (bool, error) {
	if f.markErr != nil {
		return false, f.markErr
	}
	// Simulate the DB: reading the conversation clears the viewer's unread.
	for i := range f.convList {
		f.convList[i].UnreadCount = 0
	}
	return f.markFound, nil
}

func (f *fakeConversationRepo) SearchMessages(_ context.Context, _, _ string, _ int) ([]conversation.SearchResult, error) {
	return nil, nil
}

func (f *fakeConversationRepo) UpdateMessage(_ context.Context, messageID, senderID, content string) (*conversation.Message, string, error) {
	return &conversation.Message{ID: messageID, ConversationID: validPostID, SenderID: senderID, Content: strings.TrimSpace(content), CreatedAt: time.Now(), UpdatedAt: time.Now()}, "u2", nil
}

func (f *fakeConversationRepo) DeleteMessage(_ context.Context, _ string, _ string) (string, string, time.Time, error) {
	return validPostID, "u2", time.Now(), nil
}

func (f *fakeConversationRepo) SetConversationMuted(_ context.Context, _, _ string, _ bool) (bool, error) {
	return true, nil
}

func convServer(conv *fakeConversationRepo) *http.Server {
	return convServerWithResolver(conv, nil)
}

func convServerWithResolver(conv *fakeConversationRepo, resolver languageResolver) *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	return New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{},
		&fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{},
		&fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{},
		&fakeLikeNotifier{}, &fakeCommentNotifier{}, conv,
		resolver,
	)
}

func getMessages(srv *http.Server, id, query string, withCookie bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/"+id+"/messages"+query, nil)
	if withCookie {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	}
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func mkMessage(id string, at time.Time) conversation.Message {
	return conversation.Message{ID: id, ConversationID: validPostID, SenderID: "me-id", Content: "hi", CreatedAt: at}
}

func TestListMessagesSuccess(t *testing.T) {
	conv := &fakeConversationRepo{messages: []conversation.Message{
		mkMessage("aaaaaaaa-0000-0000-0000-000000000001", time.Now()),
		mkMessage("aaaaaaaa-0000-0000-0000-000000000002", time.Now().Add(-time.Minute)),
	}}
	rec := getMessages(convServer(conv), validPostID, "", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp messageListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(resp.Items) != 2 || resp.NextCursor != "" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.Items[0].SenderID != "me-id" || resp.Items[0].Content != "hi" {
		t.Fatalf("fields not mapped: %+v", resp.Items[0])
	}
	// default limit 20 -> fetched with limit+1
	if conv.lastLimit != 21 {
		t.Fatalf("expected limit 21, got %d", conv.lastLimit)
	}
}

func TestMessageHistoryReturnsPersistedLanguageMetadata(t *testing.T) {
	for _, tc := range []struct {
		name       string
		language   string
		confidence float64
		resolution string
	}{
		{name: "current", language: "en", confidence: 0.97, resolution: "current"},
		{name: "context", language: "ru", confidence: 0.91, resolution: "context"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			language, resolution := tc.language, tc.resolution
			confidence := tc.confidence
			conv := &fakeConversationRepo{messages: []conversation.Message{{
				ID: "m-1", ConversationID: validPostID, SenderID: "me-id", Content: "hello", CreatedAt: time.Now(),
				SourceLanguage: &language, SourceLanguageConfidence: &confidence, SourceLanguageResolution: &resolution,
			}}}
			resp := getMessagesFrom(msgTranslateServer(conv, userWithTranslate(false, nil), nil), true)
			item := resp.Items[0]
			if item.SourceLanguage == nil || *item.SourceLanguage != tc.language || item.SourceLanguageConfidence == nil || *item.SourceLanguageConfidence != tc.confidence || item.SourceLanguageResolution == nil || *item.SourceLanguageResolution != tc.resolution {
				t.Fatalf("history metadata = %+v", item)
			}
		})
	}
}

func TestMessageHistoryLegacyMetadataStaysNullAndDoesNotDetect(t *testing.T) {
	conv := &fakeConversationRepo{messages: []conversation.Message{mkMessage("m-legacy", time.Now())}}
	resolver := &fakeLanguageResolver{resolution: translation.LanguageResolution{LanguageCode: "en", Confidence: 1, Source: translation.ResolutionCurrent}}
	s := msgTranslateServer(conv, userWithTranslate(false, nil), nil)
	s.languageResolver = resolver
	resp := getMessagesFrom(s, true)
	item := resp.Items[0]
	if item.SourceLanguage != nil || item.SourceLanguageConfidence != nil || item.SourceLanguageResolution != nil {
		t.Fatalf("legacy metadata was invented: %+v", item)
	}
	if resolver.calls != 0 {
		t.Fatalf("history detector calls = %d, want 0", resolver.calls)
	}
}

func TestHistoryTranslationUsesPersistedCurrentSourceAndPreferredTarget(t *testing.T) {
	language, confidence, resolution := "en", 0.97, "current"
	conv := &fakeConversationRepo{messages: []conversation.Message{{
		ID: "m-1", ConversationID: validPostID, SenderID: "me-id", Content: "hello", CreatedAt: time.Now(),
		SourceLanguage: &language, SourceLanguageConfidence: &confidence, SourceLanguageResolution: &resolution,
	}}}
	translator := &fakeTranslator{}
	me := userWithTranslate(true, func() *string { value := "es"; return &value }())
	me.PlatformLanguage = func() *string { value := "ru"; return &value }()
	me.NativeLanguage = "fr"
	resp := getMessagesFrom(msgTranslateServer(conv, me, translator), true)
	if len(translator.requests) != 1 || translator.requests[0].SourceLang != "en" || translator.requests[0].TargetLang != "es" {
		t.Fatalf("translation request = %+v", translator.requests)
	}
	if resp.Items[0].SourceLanguage == nil || *resp.Items[0].SourceLanguage != "en" {
		t.Fatalf("translated history source = %+v", resp.Items[0].SourceLanguage)
	}
}

func TestHistoryTranslationUsesPersistedContextSource(t *testing.T) {
	language, confidence, resolution := "ru", 0.91, "context"
	conv := &fakeConversationRepo{messages: []conversation.Message{{
		ID: "m-1", ConversationID: validPostID, SenderID: "me-id", Content: "привет", CreatedAt: time.Now(),
		SourceLanguage: &language, SourceLanguageConfidence: &confidence, SourceLanguageResolution: &resolution,
	}}}
	translator := &fakeTranslator{}
	resp := getMessagesFrom(msgTranslateServer(conv, userWithTranslate(true, func() *string { value := "en"; return &value }()), translator), true)
	if len(translator.requests) != 1 || translator.requests[0].SourceLang != "ru" {
		t.Fatalf("translation request = %+v", translator.requests)
	}
	if resp.Items[0].SourceLanguage == nil || *resp.Items[0].SourceLanguage != "ru" {
		t.Fatalf("translated context source = %+v", resp.Items[0].SourceLanguage)
	}
}

func TestHistoryTranslationDoesNotInventSourceForUnresolvedMetadata(t *testing.T) {
	resolution := "unresolved"
	conv := &fakeConversationRepo{messages: []conversation.Message{{
		ID: "m-1", ConversationID: validPostID, SenderID: "me-id", Content: "hello", CreatedAt: time.Now(),
		SourceLanguageResolution: &resolution,
	}}}
	translator := &fakeTranslator{}
	_ = getMessagesFrom(msgTranslateServer(conv, userWithTranslate(true, func() *string { value := "es"; return &value }()), translator), true)
	if len(translator.requests) != 1 || translator.requests[0].SourceLang != "" {
		t.Fatalf("unresolved source was supplied to v2: %+v", translator.requests)
	}
}

func TestListMessagesPaginationNextCursor(t *testing.T) {
	conv := &fakeConversationRepo{messages: []conversation.Message{
		mkMessage("aaaaaaaa-0000-0000-0000-000000000001", time.Now()),
		mkMessage("aaaaaaaa-0000-0000-0000-000000000002", time.Now().Add(-time.Minute)),
		mkMessage("aaaaaaaa-0000-0000-0000-000000000003", time.Now().Add(-2*time.Minute)),
	}}
	rec := getMessages(convServer(conv), validPostID, "?limit=2", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp messageListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.NextCursor == "" {
		t.Fatal("expected a nextCursor")
	}
	if _, ok := parseMessageCursor(resp.NextCursor); !ok {
		t.Fatal("nextCursor not decodable")
	}
}

func TestListMessagesCursorForwarded(t *testing.T) {
	conv := &fakeConversationRepo{}
	srv := convServer(conv)
	// build a valid cursor
	cursor := encodeCursor(time.Now(), "aaaaaaaa-0000-0000-0000-000000000009")
	_ = getMessages(srv, validPostID, "?cursor="+cursor, true)
	if conv.lastCur == nil {
		t.Fatal("expected cursor forwarded to repo")
	}
}

func TestListMessagesInvalidUUID(t *testing.T) {
	rec := getMessages(convServer(&fakeConversationRepo{}), "not-a-uuid", "", true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestListMessagesInvalidCursor(t *testing.T) {
	rec := getMessages(convServer(&fakeConversationRepo{}), validPostID, "?cursor=@@@", true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestListMessagesRejectsNonUUIDCursorID(t *testing.T) {
	cursor := encodeCursor(time.Now(), "not-a-uuid")
	rec := getMessages(convServer(&fakeConversationRepo{}), validPostID, "?cursor="+cursor, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestListMessagesInvalidLimit(t *testing.T) {
	rec := getMessages(convServer(&fakeConversationRepo{}), validPostID, "?limit=100", true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestListMessagesForeignNotParticipant404(t *testing.T) {
	conv := &fakeConversationRepo{listErr: conversation.ErrNotParticipant}
	rec := getMessages(convServer(conv), validPostID, "", true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestListMessagesUnauthenticated(t *testing.T) {
	rec := getMessages(convServer(&fakeConversationRepo{}), validPostID, "", false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestListMessagesRepositoryError(t *testing.T) {
	conv := &fakeConversationRepo{listErr: errForTest}
	rec := getMessages(convServer(conv), validPostID, "", true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// ---- translated message history ----

// fakeTranslator is a test double for translation.Service.
type fakeTranslator struct {
	err      error
	requests []translation.Request
}

func (f *fakeTranslator) Translate(_ context.Context, req translation.Request) (*translation.Result, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	return &translation.Result{
		TranslatedText: "[" + req.TargetLang + "] " + req.Text,
		SourceLang:     "auto",
		TargetLang:     req.TargetLang,
	}, nil
}

// msgTranslateServer builds a white-box server whose authed user is `me` and
// whose translator is `tr` (nil keeps the default stub).
func msgTranslateServer(conv *fakeConversationRepo, me *user.User, tr translation.Service) *Server {
	users := &fakeUserRepo{byID: map[string]*user.User{me.ID: me}}
	s := newServer(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users, activeSession(me.ID), &fakeFollowRepo{}, &fakeBlockRepo{},
		&fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{},
		&fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{},
		&fakeLikeNotifier{}, &fakeCommentNotifier{}, conv,
	)
	if tr != nil {
		s.translator = tr
	}
	return s
}

func getMessagesFrom(s *Server, withCookie bool) messageListResponse {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/"+validPostID+"/messages", nil)
	if withCookie {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	}
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	var resp messageListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return resp
}

func userWithTranslate(enabled bool, preferred *string) *user.User {
	u := mkUser("me-id", "me_user")
	u.AutoTranslateEnabled = enabled
	u.PreferredLanguage = preferred
	return u
}

func oneMessageConv() *fakeConversationRepo {
	return &fakeConversationRepo{messages: []conversation.Message{mkMessage("aaaaaaaa-0000-0000-0000-000000000001", time.Now())}}
}

func TestMessagesTranslateDisabled(t *testing.T) {
	es := "es"
	me := userWithTranslate(false, &es) // disabled despite a preferred language
	s := msgTranslateServer(oneMessageConv(), me, &fakeTranslator{})
	resp := getMessagesFrom(s, true)
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Items[0].TranslatedContent != nil {
		t.Fatalf("disabled: expected no translation, got %v", *resp.Items[0].TranslatedContent)
	}
	if resp.Items[0].Content != "hi" {
		t.Fatalf("original content not preserved: %q", resp.Items[0].Content)
	}
}

func TestMessagesTranslateNoPreferredLanguage(t *testing.T) {
	me := userWithTranslate(true, nil) // enabled but no target language
	s := msgTranslateServer(oneMessageConv(), me, &fakeTranslator{})
	resp := getMessagesFrom(s, true)
	if resp.Items[0].TranslatedContent != nil {
		t.Fatal("no preferred language: expected no translation")
	}
}

func TestMessagesTranslated(t *testing.T) {
	es := "es"
	me := userWithTranslate(true, &es)
	ru := "ru"
	me.PlatformLanguage = &ru
	if me.PlatformLanguage == nil || me.PreferredLanguage == nil || *me.PlatformLanguage == *me.PreferredLanguage {
		t.Fatal("platform and preferred languages must be distinct")
	}
	s := msgTranslateServer(oneMessageConv(), me, &fakeTranslator{})
	resp := getMessagesFrom(s, true)
	it := resp.Items[0]
	if it.Content != "hi" {
		t.Fatalf("original not preserved: %q", it.Content)
	}
	if it.TranslatedContent == nil || *it.TranslatedContent != "[es] hi" {
		t.Fatalf("unexpected translation: %v", it.TranslatedContent)
	}
	if it.TargetLanguage == nil || *it.TargetLanguage != "es" {
		t.Fatalf("target language missing: %v", it.TargetLanguage)
	}
	if it.SourceLanguage == nil || *it.SourceLanguage != "auto" {
		t.Fatalf("source language missing: %v", it.SourceLanguage)
	}
}

func TestMessagesTranslationErrorFallback(t *testing.T) {
	es := "es"
	me := userWithTranslate(true, &es)
	s := msgTranslateServer(oneMessageConv(), me, &fakeTranslator{err: errForTest})
	resp := getMessagesFrom(s, true)
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item even on translation error, got %d", len(resp.Items))
	}
	if resp.Items[0].TranslatedContent != nil {
		t.Fatal("translation error: fields must stay nil")
	}
	if resp.Items[0].Content != "hi" {
		t.Fatalf("original content must survive a translation error: %q", resp.Items[0].Content)
	}
}

func TestMessagesTranslateUnauthenticated(t *testing.T) {
	es := "es"
	me := userWithTranslate(true, &es)
	s := msgTranslateServer(oneMessageConv(), me, &fakeTranslator{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/"+validPostID+"/messages", nil)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// ---- list conversations ----

func getConversations(srv *http.Server, query string, withCookie bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/conversations"+query, nil)
	if withCookie {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	}
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func mkConvItem(id string, at time.Time, withMsg bool) conversation.ListItem {
	it := conversation.ListItem{
		ID: id, UpdatedAt: at,
		OtherID: "u2", OtherUsername: "bob", OtherDisplayName: "Bob",
	}
	if withMsg {
		mid, sid, content := "m-1", "u2", "hi there"
		it.LastMessageID = &mid
		it.LastMessageSenderID = &sid
		it.LastMessageContent = &content
		it.LastMessageCreatedAt = &at
		it.UnreadCount = 2
	}
	return it
}

func TestListConversationsSuccess(t *testing.T) {
	conv := &fakeConversationRepo{convList: []conversation.ListItem{
		mkConvItem("c2", time.Now(), true),
		mkConvItem("c1", time.Now().Add(-time.Hour), false),
	}}
	rec := getConversations(convServer(conv), "", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp conversationListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(resp.Items) != 2 || resp.NextCursor != "" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.Items[0].OtherUser.Username != "bob" {
		t.Fatalf("other participant not mapped: %+v", resp.Items[0])
	}
	if resp.Items[0].LastMessage == nil || resp.Items[0].LastMessage.Content != "hi there" {
		t.Fatalf("latest message not mapped: %+v", resp.Items[0])
	}
	if resp.Items[1].LastMessage != nil {
		t.Fatalf("expected nil last message for c1: %+v", resp.Items[1])
	}
	if conv.lastLimit != 21 {
		t.Fatalf("expected limit 21, got %d", conv.lastLimit)
	}
}

func TestListConversationsEmpty(t *testing.T) {
	rec := getConversations(convServer(&fakeConversationRepo{}), "", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp conversationListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 0 || resp.NextCursor != "" {
		t.Fatalf("expected empty list, got %+v", resp)
	}
}

func TestListConversationsPaginationNextCursor(t *testing.T) {
	conv := &fakeConversationRepo{convList: []conversation.ListItem{
		mkConvItem("33333333-3333-3333-3333-333333333333", time.Now(), true),
		mkConvItem("22222222-2222-2222-2222-222222222222", time.Now().Add(-time.Minute), true),
		mkConvItem("11111111-1111-1111-1111-111111111111", time.Now().Add(-time.Hour), true),
	}}
	rec := getConversations(convServer(conv), "?limit=2", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp conversationListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.NextCursor == "" {
		t.Fatal("expected a nextCursor")
	}
	if _, ok := parseConversationCursor(resp.NextCursor); !ok {
		t.Fatal("nextCursor not decodable")
	}
}

func TestListConversationsRejectsNonUUIDCursorID(t *testing.T) {
	cursor := encodeCursor(time.Now(), "not-a-uuid")
	rec := getConversations(convServer(&fakeConversationRepo{}), "?cursor="+cursor, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestListConversationsInvalidLimit(t *testing.T) {
	rec := getConversations(convServer(&fakeConversationRepo{}), "?limit=100", true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestListConversationsUnauthenticated(t *testing.T) {
	rec := getConversations(convServer(&fakeConversationRepo{}), "", false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestListConversationsRepositoryError(t *testing.T) {
	rec := getConversations(convServer(&fakeConversationRepo{convListErr: errForTest}), "", true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestListConversationsExposesUnreadCount(t *testing.T) {
	conv := &fakeConversationRepo{convList: []conversation.ListItem{mkConvItem("c1", time.Now(), true)}}
	rec := getConversations(convServer(conv), "", true)
	var resp conversationListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 1 || resp.Items[0].UnreadCount != 2 {
		t.Fatalf("unreadCount not exposed: %+v", resp.Items)
	}
}

// ---- mark conversation read ----

func markConvRead(srv *http.Server, id string, withCookie, withOrigin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+id+"/read", nil)
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

func TestMarkConversationReadSuccess(t *testing.T) {
	rec := markConvRead(convServer(&fakeConversationRepo{markFound: true}), validPostID, true, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestMarkConversationReadRepeatIdempotent(t *testing.T) {
	srv := convServer(&fakeConversationRepo{markFound: true})
	_ = markConvRead(srv, validPostID, true, true)
	rec := markConvRead(srv, validPostID, true, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("repeat should be 204, got %d", rec.Code)
	}
}

func TestMarkConversationReadForeign404(t *testing.T) {
	rec := markConvRead(convServer(&fakeConversationRepo{markFound: false}), validPostID, true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestMarkConversationReadInvalidUUID(t *testing.T) {
	rec := markConvRead(convServer(&fakeConversationRepo{markFound: true}), "not-a-uuid", true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestMarkConversationReadUnauthenticated(t *testing.T) {
	rec := markConvRead(convServer(&fakeConversationRepo{markFound: true}), validPostID, false, true)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMarkConversationReadMissingOrigin(t *testing.T) {
	rec := markConvRead(convServer(&fakeConversationRepo{markFound: true}), validPostID, true, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestMarkConversationReadRepositoryError(t *testing.T) {
	rec := markConvRead(convServer(&fakeConversationRepo{markErr: errForTest}), validPostID, true, true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestConversationUnreadCountBeforeAndAfterRead(t *testing.T) {
	conv := &fakeConversationRepo{markFound: true, convList: []conversation.ListItem{mkConvItem("c1", time.Now(), true)}}
	srv := convServer(conv)

	// before: unread reflects unseen messages
	var before conversationListResponse
	_ = json.Unmarshal(getConversations(srv, "", true).Body.Bytes(), &before)
	if before.Items[0].UnreadCount != 2 {
		t.Fatalf("expected unread 2 before read, got %d", before.Items[0].UnreadCount)
	}

	// mark read
	if rec := markConvRead(srv, validPostID, true, true); rec.Code != http.StatusNoContent {
		t.Fatalf("mark read expected 204, got %d", rec.Code)
	}

	// after: unread cleared
	var after conversationListResponse
	_ = json.Unmarshal(getConversations(srv, "", true).Body.Bytes(), &after)
	if after.Items[0].UnreadCount != 0 {
		t.Fatalf("expected unread 0 after read, got %d", after.Items[0].UnreadCount)
	}
}

// ---- create message ----

func postMessage(srv *http.Server, id, body string, withCookie, withOrigin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+id+"/messages", bytes.NewBufferString(body))
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

func TestCreateMessageSuccess(t *testing.T) {
	conv := &fakeConversationRepo{}
	rec := postMessage(convServer(conv), validPostID, `{"content":"hello"}`, true, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp messageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if resp.ID != "m-1" || resp.Content != "hello" || resp.SenderID != "me-id" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	// sender must be the authenticated user, never client-supplied
	if conv.lastSender != "me-id" {
		t.Fatalf("sender not the auth user: %q", conv.lastSender)
	}
}

func TestAttachmentOnlySkipsLanguageDetection(t *testing.T) {
	resolver := &fakeLanguageResolver{}
	s := newServer(config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin}, fakePinger{}, &fakeUserRepo{byID: map[string]*user.User{"me-id": mkUser("me-id", "me_user")}}, activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{}, &fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{}, resolver)
	s.resolveCreatedMessageLanguage(context.Background(), &conversation.Message{Content: ""}, "me-id")
	if resolver.calls != 0 {
		t.Fatalf("attachment-only message invoked detector %d times", resolver.calls)
	}
}

type fakeLanguageResolver struct {
	resolution translation.LanguageResolution
	err        error
	calls      int
	current    string
	context    string
}

func (f *fakeLanguageResolver) Resolve(_ context.Context, currentText, contextText string) (translation.LanguageResolution, error) {
	f.calls++
	f.current = currentText
	f.context = contextText
	return f.resolution, f.err
}

func TestCreateMessageLanguageCurrentMetadata(t *testing.T) {
	conv := &fakeConversationRepo{contextText: "older message"}
	resolver := &fakeLanguageResolver{resolution: translation.LanguageResolution{
		LanguageCode: "en", Confidence: 0.97, Source: translation.ResolutionCurrent,
	}}
	var resp messageResponse
	rec := postMessage(convServerWithResolver(conv, resolver), validPostID, `{"content":"hello"}`, true, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if resolver.calls != 1 || resolver.current != "hello" || resolver.context != "older message" {
		t.Fatalf("resolver calls/input = %d/%q/%q", resolver.calls, resolver.current, resolver.context)
	}
	if conv.metadataCalls != 1 || conv.metadataLanguage == nil || *conv.metadataLanguage != "en" || conv.metadataConfidence == nil || *conv.metadataConfidence != 0.97 || conv.metadataResolution != "current" {
		t.Fatalf("metadata persistence = calls:%d language:%v confidence:%v resolution:%q", conv.metadataCalls, conv.metadataLanguage, conv.metadataConfidence, conv.metadataResolution)
	}
	if resp.SourceLanguage == nil || *resp.SourceLanguage != "en" || resp.SourceLanguageConfidence == nil || *resp.SourceLanguageConfidence != 0.97 || resp.SourceLanguageResolution == nil || *resp.SourceLanguageResolution != "current" {
		t.Fatalf("response metadata = %+v", resp)
	}
}

func TestCreateMessageLanguageContextMetadata(t *testing.T) {
	conv := &fakeConversationRepo{contextText: "older message"}
	resolver := &fakeLanguageResolver{resolution: translation.LanguageResolution{
		LanguageCode: "ru", Confidence: 0.91, Source: translation.ResolutionContext,
	}}
	if rec := postMessage(convServerWithResolver(conv, resolver), validPostID, `{"content":"short"}`, true, true); rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if conv.metadataLanguage == nil || *conv.metadataLanguage != "ru" || conv.metadataResolution != "context" {
		t.Fatalf("context metadata not persisted: language=%v resolution=%q", conv.metadataLanguage, conv.metadataResolution)
	}
}

func TestCreateMessageLanguageUnresolvedMetadata(t *testing.T) {
	conv := &fakeConversationRepo{}
	resolver := &fakeLanguageResolver{resolution: translation.LanguageResolution{Source: translation.ResolutionUnresolved}}
	var resp messageResponse
	rec := postMessage(convServerWithResolver(conv, resolver), validPostID, `{"content":"unknown"}`, true, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if conv.metadataLanguage != nil || conv.metadataConfidence != nil || conv.metadataResolution != "unresolved" {
		t.Fatalf("unresolved metadata invented values: language=%v confidence=%v resolution=%q", conv.metadataLanguage, conv.metadataConfidence, conv.metadataResolution)
	}
	if resp.SourceLanguage != nil || resp.SourceLanguageConfidence != nil || resp.SourceLanguageResolution == nil || *resp.SourceLanguageResolution != "unresolved" {
		t.Fatalf("unresolved response metadata = %+v", resp)
	}
}

func TestCreateMessageLanguageFailuresAreNonBlocking(t *testing.T) {
	tests := []struct {
		name        string
		resolver    languageResolver
		contextErr  error
		metadataErr error
		wantCalls   int
	}{
		{name: "disabled", wantCalls: 0},
		{name: "detector error", resolver: &fakeLanguageResolver{err: errForTest}, wantCalls: 1},
		{name: "context error", resolver: &fakeLanguageResolver{}, contextErr: errForTest, wantCalls: 0},
		{name: "metadata error", resolver: &fakeLanguageResolver{resolution: translation.LanguageResolution{LanguageCode: "en", Confidence: 0.9, Source: translation.ResolutionCurrent}}, metadataErr: errForTest, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := &fakeConversationRepo{contextErr: tt.contextErr, metadataErr: tt.metadataErr}
			rec := postMessage(convServerWithResolver(conv, tt.resolver), validPostID, `{"content":"hello"}`, true, true)
			if rec.Code != http.StatusCreated {
				t.Fatalf("expected 201 despite enrichment failure, got %d", rec.Code)
			}
			if tt.resolver != nil {
				if got := tt.resolver.(*fakeLanguageResolver).calls; got != tt.wantCalls {
					t.Fatalf("resolver calls = %d, want %d", got, tt.wantCalls)
				}
			}
		})
	}
}

func TestCreateMessageSkipsEmptyTextDetection(t *testing.T) {
	conv := &fakeConversationRepo{}
	resolver := &fakeLanguageResolver{resolution: translation.LanguageResolution{LanguageCode: "en", Confidence: 1, Source: translation.ResolutionCurrent}}
	s := convDeliveryServerWith(conv, recipientUser("u2", false, nil), &fakeTranslator{})
	s.languageResolver = resolver
	s.resolveCreatedMessageLanguage(context.Background(), &conversation.Message{ID: "m-1", ConversationID: validPostID, Content: "   "}, "me-id")
	if resolver.calls != 0 || conv.metadataCalls != 0 {
		t.Fatalf("empty text enrichment calls = resolver:%d metadata:%d", resolver.calls, conv.metadataCalls)
	}
}

func TestCreateMessageBlockedReturnsForbidden(t *testing.T) {
	conv := &fakeConversationRepo{createErr: conversation.ErrBlocked}
	rec := postMessage(convServer(conv), validPostID, `{"content":"hello"}`, true, true)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "interaction not allowed") {
		t.Fatalf("unexpected error body: %s", rec.Body.String())
	}
}

func TestCreateMessageTrim(t *testing.T) {
	conv := &fakeConversationRepo{}
	rec := postMessage(convServer(conv), validPostID, `{"content":"  hi  "}`, true, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	var resp messageResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Content != "hi" {
		t.Fatalf("content not trimmed in response: %q", resp.Content)
	}
}

func TestCreateMessageEmpty(t *testing.T) {
	conv := &fakeConversationRepo{createErr: conversation.ErrEmptyContent}
	rec := postMessage(convServer(conv), validPostID, `{"content":"   "}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreateMessageTooLong(t *testing.T) {
	conv := &fakeConversationRepo{createErr: conversation.ErrContentTooLong}
	rec := postMessage(convServer(conv), validPostID, `{"content":"long"}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreateMessageInvalidUUID(t *testing.T) {
	rec := postMessage(convServer(&fakeConversationRepo{}), "not-a-uuid", `{"content":"hi"}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreateMessageForeignNotParticipant404(t *testing.T) {
	conv := &fakeConversationRepo{createErr: conversation.ErrNotParticipant}
	rec := postMessage(convServer(conv), validPostID, `{"content":"hi"}`, true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestCreateMessageUnauthenticated(t *testing.T) {
	rec := postMessage(convServer(&fakeConversationRepo{}), validPostID, `{"content":"hi"}`, false, true)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCreateMessageMissingOrigin(t *testing.T) {
	rec := postMessage(convServer(&fakeConversationRepo{}), validPostID, `{"content":"hi"}`, true, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestCreateMessageRepositoryError(t *testing.T) {
	conv := &fakeConversationRepo{createErr: errForTest}
	rec := postMessage(convServer(conv), validPostID, `{"content":"hi"}`, true, true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// ---- translated create-message response (sender prefs) ----

// postMessageResp posts a message to a white-box server and decodes the body.
func postMessageResp(t *testing.T, s *Server) messageResponse {
	t.Helper()
	rec := postMessageTo(s, validPostID, `{"content":"hola"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp messageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	return resp
}

func TestCreateMessageResponseTranslateDisabled(t *testing.T) {
	es := "es"
	s := msgTranslateServer(&fakeConversationRepo{recipientID: "u2"}, userWithTranslate(false, &es), &fakeTranslator{})
	resp := postMessageResp(t, s)
	if resp.TranslatedContent != nil {
		t.Fatal("disabled: expected no translation")
	}
	if resp.Content != "hola" {
		t.Fatalf("original content not preserved: %q", resp.Content)
	}
}

func TestCreateMessageResponseTranslated(t *testing.T) {
	es := "es"
	s := msgTranslateServer(&fakeConversationRepo{recipientID: "u2"}, userWithTranslate(true, &es), &fakeTranslator{})
	resp := postMessageResp(t, s)
	if resp.Content != "hola" {
		t.Fatalf("original not preserved: %q", resp.Content)
	}
	if resp.TranslatedContent == nil || *resp.TranslatedContent != "[es] hola" {
		t.Fatalf("unexpected translation: %v", resp.TranslatedContent)
	}
	if resp.TargetLanguage == nil || *resp.TargetLanguage != "es" {
		t.Fatalf("target language missing: %v", resp.TargetLanguage)
	}
	if resp.SourceLanguage == nil || *resp.SourceLanguage != "auto" {
		t.Fatalf("source language missing: %v", resp.SourceLanguage)
	}
}

func TestCreateMessageResponseNoPreferredLanguage(t *testing.T) {
	s := msgTranslateServer(&fakeConversationRepo{recipientID: "u2"}, userWithTranslate(true, nil), &fakeTranslator{})
	resp := postMessageResp(t, s)
	if resp.TranslatedContent != nil {
		t.Fatal("no preferred language: expected no translation")
	}
}

func TestCreateMessageResponseTranslationErrorFallback(t *testing.T) {
	es := "es"
	s := msgTranslateServer(&fakeConversationRepo{recipientID: "u2"}, userWithTranslate(true, &es), &fakeTranslator{err: errForTest})
	resp := postMessageResp(t, s)
	if resp.TranslatedContent != nil {
		t.Fatal("translation error: fields must stay nil")
	}
	if resp.Content != "hola" {
		t.Fatalf("original must survive translation error: %q", resp.Content)
	}
}

// ---- realtime delivery ----

// convDeliveryServer builds a white-box server so tests can reach the hub and
// register recipient connections directly.
func convDeliveryServer(conv *fakeConversationRepo) *Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	return newServer(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{},
		&fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{},
		&fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{},
		&fakeLikeNotifier{}, &fakeCommentNotifier{}, conv,
	)
}

func postMessageTo(s *Server, id, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+id+"/messages", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	req.Header.Set("Origin", testOrigin)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	return rec
}

func TestRealtimeRecipientReceivesEvent(t *testing.T) {
	conv := &fakeConversationRepo{recipientID: "u2"}
	s := convDeliveryServer(conv)

	recipient := realtime.NewClient("u2", 4)
	s.hub.Register(recipient)

	rec := postMessageTo(s, validPostID, `{"content":"hi there"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	select {
	case raw := <-recipient.Send():
		var ev struct {
			Type string `json:"type"`
			Data struct {
				ConversationID    string  `json:"conversationId"`
				ID                string  `json:"id"`
				SenderID          string  `json:"senderId"`
				Content           string  `json:"content"`
				CreatedAt         string  `json:"createdAt"`
				TranslatedContent *string `json:"translatedContent"`
				SourceLanguage    *string `json:"sourceLanguage"`
				TargetLanguage    *string `json:"targetLanguage"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &ev); err != nil {
			t.Fatalf("invalid event json: %v", err)
		}
		if ev.Type != "message.created" {
			t.Fatalf("unexpected event type: %q", ev.Type)
		}
		// New field: conversation id must be present and correct.
		if ev.Data.ConversationID != validPostID {
			t.Fatalf("expected conversationId %q, got %q", validPostID, ev.Data.ConversationID)
		}
		// Existing fields must remain intact.
		if ev.Data.ID == "" || ev.Data.CreatedAt == "" {
			t.Fatalf("missing existing fields: %+v", ev.Data)
		}
		if ev.Data.Content != "hi there" || ev.Data.SenderID != "me-id" {
			t.Fatalf("unexpected event data: %+v", ev.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("recipient did not receive the event")
	}
}

func TestRealtimeAttachmentPayloadUsesPersistedMetadata(t *testing.T) {
	s := convDeliveryServer(&fakeConversationRepo{})
	recipient := realtime.NewClient("u2", 2)
	s.hub.Register(recipient)
	s.publishMessageCreated(context.Background(), validPostID, "u2", messageResponse{ID: "m1", SenderID: "me-id", Content: "", CreatedAt: time.Now().Format(time.RFC3339), Attachment: &messageAttachmentResponse{ID: "a1", Filename: "voice.webm", Type: "voice", MimeType: "audio/webm", SizeBytes: 42, URL: "https://signed.example/file"}})
	raw := <-recipient.Send()
	var ev struct {
		Data struct {
			Attachment *messageAttachmentResponse `json:"attachment"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Data.Attachment == nil || ev.Data.Attachment.Filename != "voice.webm" || ev.Data.Attachment.Type != "voice" || ev.Data.Attachment.MimeType != "audio/webm" || ev.Data.Attachment.SizeBytes != 42 {
		t.Fatalf("attachment metadata missing from realtime payload: %+v", ev.Data.Attachment)
	}
}

func TestRealtimeUnrelatedUserDoesNotReceive(t *testing.T) {
	conv := &fakeConversationRepo{recipientID: "u2"}
	s := convDeliveryServer(conv)

	recipient := realtime.NewClient("u2", 4)
	stranger := realtime.NewClient("u9", 4)
	s.hub.Register(recipient)
	s.hub.Register(stranger)

	if rec := postMessageTo(s, validPostID, `{"content":"hi"}`); rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	select {
	case <-stranger.Send():
		t.Fatal("unrelated user must not receive the event")
	default:
	}
	// recipient still got it
	select {
	case <-recipient.Send():
	default:
		t.Fatal("recipient should have received the event")
	}
}

func TestRealtimeNoEventOnPersistenceError(t *testing.T) {
	conv := &fakeConversationRepo{recipientID: "u2", createErr: errForTest}
	s := convDeliveryServer(conv)

	recipient := realtime.NewClient("u2", 4)
	s.hub.Register(recipient)

	if rec := postMessageTo(s, validPostID, `{"content":"hi"}`); rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	select {
	case <-recipient.Send():
		t.Fatal("no event must be published when persistence fails")
	default:
	}
}

func TestRealtimeNoEventOnBlockedMessage(t *testing.T) {
	conv := &fakeConversationRepo{recipientID: "u2", createErr: conversation.ErrBlocked}
	s := convDeliveryServer(conv)

	recipient := realtime.NewClient("u2", 4)
	s.hub.Register(recipient)

	if rec := postMessageTo(s, validPostID, `{"content":"hi"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
	select {
	case <-recipient.Send():
		t.Fatal("blocked message must not publish a realtime event")
	default:
	}
}

func TestRealtimeDisconnectedRecipientDoesNotBreakRequest(t *testing.T) {
	// No recipient connection is registered; the request must still succeed.
	conv := &fakeConversationRepo{recipientID: "u2"}
	s := convDeliveryServer(conv)
	if rec := postMessageTo(s, validPostID, `{"content":"hi"}`); rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 with no recipient connection, got %d", rec.Code)
	}
}

// ---- realtime translated event (recipient prefs) ----

// convDeliveryServerWith registers the recipient user (with its prefs) and a
// translator so the publish path can translate for the recipient.
func convDeliveryServerWith(conv *fakeConversationRepo, recipient *user.User, tr translation.Service) *Server {
	me := mkUser("me-id", "me_user")
	byID := map[string]*user.User{"me-id": me}
	if recipient != nil {
		byID[recipient.ID] = recipient
	}
	users := &fakeUserRepo{byID: byID}
	s := newServer(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{},
		&fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{},
		&fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{},
		&fakeLikeNotifier{}, &fakeCommentNotifier{}, conv,
	)
	if tr != nil {
		s.translator = tr
	}
	return s
}

func recipientUser(id string, enabled bool, preferred *string) *user.User {
	u := mkUser(id, id+"_user")
	u.AutoTranslateEnabled = enabled
	u.PreferredLanguage = preferred
	return u
}

// publishAndRead posts a message and returns the event the recipient receives.
func publishAndRead(t *testing.T, s *Server) struct {
	Type string          `json:"type"`
	Data messageResponse `json:"data"`
} {
	t.Helper()
	recipient := realtime.NewClient("u2", 4)
	s.hub.Register(recipient)
	if rec := postMessageTo(s, validPostID, `{"content":"hi there"}`); rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	var ev struct {
		Type string          `json:"type"`
		Data messageResponse `json:"data"`
	}
	select {
	case raw := <-recipient.Send():
		if err := json.Unmarshal(raw, &ev); err != nil {
			t.Fatalf("invalid event json: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("recipient did not receive the event")
	}
	return ev
}

func TestRealtimeEventTranslateDisabled(t *testing.T) {
	es := "es"
	s := convDeliveryServerWith(&fakeConversationRepo{recipientID: "u2"}, recipientUser("u2", false, &es), &fakeTranslator{})
	ev := publishAndRead(t, s)
	if ev.Data.TranslatedContent != nil {
		t.Fatal("disabled recipient: expected no translation")
	}
	if ev.Data.Content != "hi there" {
		t.Fatalf("original content not preserved: %q", ev.Data.Content)
	}
}

func TestRealtimeEventTranslated(t *testing.T) {
	es := "es"
	s := convDeliveryServerWith(&fakeConversationRepo{recipientID: "u2"}, recipientUser("u2", true, &es), &fakeTranslator{})
	ev := publishAndRead(t, s)
	if ev.Data.Content != "hi there" {
		t.Fatalf("original not preserved: %q", ev.Data.Content)
	}
	if ev.Data.TranslatedContent == nil || *ev.Data.TranslatedContent != "[es] hi there" {
		t.Fatalf("unexpected translation: %v", ev.Data.TranslatedContent)
	}
	if ev.Data.TargetLanguage == nil || *ev.Data.TargetLanguage != "es" {
		t.Fatalf("target language missing: %v", ev.Data.TargetLanguage)
	}
	if ev.Data.SourceLanguage == nil || *ev.Data.SourceLanguage != "auto" {
		t.Fatalf("source language missing: %v", ev.Data.SourceLanguage)
	}
}

func TestRealtimeEventReusesCreatedMessageLanguageMetadata(t *testing.T) {
	conv := &fakeConversationRepo{}
	resolver := &fakeLanguageResolver{resolution: translation.LanguageResolution{
		LanguageCode: "en", Confidence: 0.96, Source: translation.ResolutionCurrent,
	}}
	s := convDeliveryServerWith(conv, recipientUser("u2", false, nil), &fakeTranslator{})
	s.languageResolver = resolver
	ev := publishAndRead(t, s)
	if resolver.calls != 1 {
		t.Fatalf("resolver calls = %d, want 1", resolver.calls)
	}
	if conv.metadataCalls != 1 {
		t.Fatalf("metadata persistence calls = %d, want 1", conv.metadataCalls)
	}
	if ev.Data.SourceLanguage == nil || *ev.Data.SourceLanguage != "en" || ev.Data.SourceLanguageConfidence == nil || *ev.Data.SourceLanguageConfidence != 0.96 || ev.Data.SourceLanguageResolution == nil || *ev.Data.SourceLanguageResolution != "current" {
		t.Fatalf("realtime metadata = %+v", ev.Data)
	}
}

func TestRealtimeEventNoPreferredLanguage(t *testing.T) {
	s := convDeliveryServerWith(&fakeConversationRepo{recipientID: "u2"}, recipientUser("u2", true, nil), &fakeTranslator{})
	ev := publishAndRead(t, s)
	if ev.Data.TranslatedContent != nil {
		t.Fatal("no preferred language: expected no translation")
	}
}

func TestRealtimeEventTranslationErrorDelivers(t *testing.T) {
	es := "es"
	s := convDeliveryServerWith(&fakeConversationRepo{recipientID: "u2"}, recipientUser("u2", true, &es), &fakeTranslator{err: errForTest})
	ev := publishAndRead(t, s)
	if ev.Type != "message.created" {
		t.Fatalf("event not delivered: %+v", ev)
	}
	if ev.Data.TranslatedContent != nil {
		t.Fatal("translation error: fields must stay nil")
	}
	if ev.Data.Content != "hi there" {
		t.Fatalf("original must survive translation error: %q", ev.Data.Content)
	}
}
