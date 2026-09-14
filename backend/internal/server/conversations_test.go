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
	openConv    *conversation.Conversation
	openErr     error
	messages    []conversation.Message
	listErr     error
	lastCur     *conversation.MessageCursor
	lastLimit   int
	createErr   error
	recipientID string
	lastContent string
	lastSender  string
	convList    []conversation.ListItem
	convListErr error
	lastConvCur *conversation.ConversationCursor
	markFound   bool
	markErr     error
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
	return &conversation.Message{ID: "m-1", ConversationID: conversationID, SenderID: senderID, Content: strings.TrimSpace(content), CreatedAt: time.Now()}, recipient, nil
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

func convServer(conv *fakeConversationRepo) *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	return New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{},
		&fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{},
		&fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{},
		&fakeLikeNotifier{}, &fakeCommentNotifier{}, conv,
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
	err error
}

func (f *fakeTranslator) Translate(_ context.Context, req translation.Request) (*translation.Result, error) {
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
		mkConvItem("c3", time.Now(), true),
		mkConvItem("c2", time.Now().Add(-time.Minute), true),
		mkConvItem("c1", time.Now().Add(-time.Hour), true),
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
