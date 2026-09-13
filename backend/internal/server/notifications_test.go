package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"together/backend/internal/config"
	"together/backend/internal/notification"
	"together/backend/internal/user"
)

// fakeNotificationRepo is a test double for notification.Repository.
type fakeNotificationRepo struct {
	created      *notification.Notification
	createErr    error
	unread       int64
	unreadErr    error
	list         []notification.ListItem
	listErr      error
	lastCur      *notification.Cursor
	lastLimit    int
	markFound    bool
	markErr      error
	markCalls    [][2]string
	markAllErr   error
	markAllCalls []string
}

func (f *fakeNotificationRepo) Create(_ context.Context, in notification.CreateInput) (*notification.Notification, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = &notification.Notification{UserID: in.UserID, ActorID: in.ActorID, Type: in.Type}
	return f.created, nil
}

func (f *fakeNotificationRepo) CountUnread(_ context.Context, _ string) (int64, error) {
	return f.unread, f.unreadErr
}

func (f *fakeNotificationRepo) List(_ context.Context, _ string, cur *notification.Cursor, limit int) ([]notification.ListItem, error) {
	f.lastCur = cur
	f.lastLimit = limit
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.list, nil
}

func (f *fakeNotificationRepo) MarkRead(_ context.Context, userID, notificationID string) (bool, error) {
	f.markCalls = append(f.markCalls, [2]string{userID, notificationID})
	if f.markErr != nil {
		return false, f.markErr
	}
	return f.markFound, nil
}

func (f *fakeNotificationRepo) MarkAllRead(_ context.Context, userID string) error {
	f.markAllCalls = append(f.markAllCalls, userID)
	return f.markAllErr
}

func notificationServer(notifs notification.Repository) *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	return New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{},
		&fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, &fakeStorageRepo{},
		&fakeBookmarkRepo{}, notifs, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
}

func getNotifications(srv *http.Server, query string, withCookie bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications"+query, nil)
	if withCookie {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	}
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func mkNotification(id string) notification.ListItem {
	actorID := "actor-id"
	username := "actor_user"
	display := "Actor"
	return notification.ListItem{
		ID: id, Type: notification.TypeFollow, CreatedAt: time.Now(),
		ActorID: &actorID, ActorUsername: &username, ActorDisplayName: &display,
	}
}

func TestNotificationsList(t *testing.T) {
	notifs := &fakeNotificationRepo{list: []notification.ListItem{
		mkNotification("11111111-1111-1111-1111-111111111111"),
		mkNotification("22222222-2222-2222-2222-222222222222"),
	}}
	rec := getNotifications(notificationServer(notifs), "", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp notificationListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.NextCursor != "" {
		t.Fatalf("expected empty nextCursor, got %q", resp.NextCursor)
	}
	if resp.Items[0].Actor == nil || resp.Items[0].Actor.Username != "actor_user" {
		t.Fatalf("actor fields missing: %+v", resp.Items[0])
	}
}

func TestNotificationsDefaultLimit(t *testing.T) {
	notifs := &fakeNotificationRepo{}
	_ = getNotifications(notificationServer(notifs), "", true)
	// default 20, fetched with limit+1
	if notifs.lastLimit != 21 {
		t.Fatalf("expected limit 21 (default 20 + 1), got %d", notifs.lastLimit)
	}
}

func TestNotificationsPaginationNextCursor(t *testing.T) {
	notifs := &fakeNotificationRepo{list: []notification.ListItem{
		mkNotification("11111111-1111-1111-1111-111111111111"),
		mkNotification("22222222-2222-2222-2222-222222222222"),
		mkNotification("33333333-3333-3333-3333-333333333333"),
	}}
	rec := getNotifications(notificationServer(notifs), "?limit=2", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp notificationListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.NextCursor == "" {
		t.Fatal("expected a nextCursor")
	}
	if _, ok := parseNotificationCursor(resp.NextCursor); !ok {
		t.Fatal("nextCursor not decodable")
	}
}

func TestNotificationsCursorPassedToRepo(t *testing.T) {
	first := &fakeNotificationRepo{list: []notification.ListItem{
		mkNotification("11111111-1111-1111-1111-111111111111"),
		mkNotification("22222222-2222-2222-2222-222222222222"),
		mkNotification("33333333-3333-3333-3333-333333333333"),
	}}
	srv := notificationServer(first)
	rec := getNotifications(srv, "?limit=2", true)
	var resp notificationListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)

	next := &fakeNotificationRepo{}
	_ = getNotifications(notificationServer(next), "?limit=2&cursor="+resp.NextCursor, true)
	if next.lastCur == nil {
		t.Fatal("expected cursor forwarded to repo")
	}
}

func TestNotificationsInvalidCursor(t *testing.T) {
	rec := getNotifications(notificationServer(&fakeNotificationRepo{}), "?cursor=@@@", true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestNotificationsInvalidLimit(t *testing.T) {
	rec := getNotifications(notificationServer(&fakeNotificationRepo{}), "?limit=100", true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestNotificationsUnauthenticated(t *testing.T) {
	rec := getNotifications(notificationServer(&fakeNotificationRepo{}), "", false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestNotificationsRepositoryError(t *testing.T) {
	rec := getNotifications(notificationServer(&fakeNotificationRepo{listErr: errForTest}), "", true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestNotificationsActorNilWhenRemoved(t *testing.T) {
	it := notification.ListItem{ID: "11111111-1111-1111-1111-111111111111", Type: notification.TypeFollow, CreatedAt: time.Now()}
	notifs := &fakeNotificationRepo{list: []notification.ListItem{it}}
	rec := getNotifications(notificationServer(notifs), "", true)
	var resp notificationListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 1 || resp.Items[0].Actor != nil {
		t.Fatalf("expected nil actor when actor removed: %+v", resp.Items)
	}
}

// ---- unread count ----

func getUnreadCount(srv *http.Server, withCookie bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/unread-count", nil)
	if withCookie {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	}
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func TestUnreadCountSuccess(t *testing.T) {
	rec := getUnreadCount(notificationServer(&fakeNotificationRepo{unread: 5}), true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if resp["unreadCount"].(float64) != 5 {
		t.Fatalf("expected unreadCount 5, got %v", resp["unreadCount"])
	}
}

func TestUnreadCountZero(t *testing.T) {
	rec := getUnreadCount(notificationServer(&fakeNotificationRepo{unread: 0}), true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["unreadCount"].(float64) != 0 {
		t.Fatalf("expected unreadCount 0, got %v", resp["unreadCount"])
	}
}

func TestUnreadCountUnauthenticated(t *testing.T) {
	rec := getUnreadCount(notificationServer(&fakeNotificationRepo{unread: 5}), false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestUnreadCountRepositoryError(t *testing.T) {
	rec := getUnreadCount(notificationServer(&fakeNotificationRepo{unreadErr: errForTest}), true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// ---- mark read ----

func markRead(srv *http.Server, id string, withCookie, withOrigin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/"+id+"/read", nil)
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

func TestMarkReadOwn(t *testing.T) {
	notifs := &fakeNotificationRepo{markFound: true}
	rec := markRead(notificationServer(notifs), validPostID, true, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d (%s)", rec.Code, rec.Body.String())
	}
	if len(notifs.markCalls) != 1 || notifs.markCalls[0] != [2]string{"me-id", validPostID} {
		t.Fatalf("unexpected mark calls: %v", notifs.markCalls)
	}
}

func TestMarkReadRepeatIdempotent(t *testing.T) {
	notifs := &fakeNotificationRepo{markFound: true}
	srv := notificationServer(notifs)
	_ = markRead(srv, validPostID, true, true)
	rec := markRead(srv, validPostID, true, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("repeat should be 204, got %d", rec.Code)
	}
}

func TestMarkReadForeign404(t *testing.T) {
	// repo reports no matching row for this user -> 404
	notifs := &fakeNotificationRepo{markFound: false}
	rec := markRead(notificationServer(notifs), validPostID, true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestMarkReadUnknown404(t *testing.T) {
	notifs := &fakeNotificationRepo{markFound: false}
	rec := markRead(notificationServer(notifs), validPostID, true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestMarkReadMalformedUUID(t *testing.T) {
	rec := markRead(notificationServer(&fakeNotificationRepo{markFound: true}), "not-a-uuid", true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestMarkReadUnauthenticated(t *testing.T) {
	rec := markRead(notificationServer(&fakeNotificationRepo{markFound: true}), validPostID, false, true)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMarkReadMissingOrigin(t *testing.T) {
	rec := markRead(notificationServer(&fakeNotificationRepo{markFound: true}), validPostID, true, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestMarkReadRepositoryError(t *testing.T) {
	rec := markRead(notificationServer(&fakeNotificationRepo{markErr: errForTest}), validPostID, true, true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// ---- mark all read ----

func markAllRead(srv *http.Server, withCookie, withOrigin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/read-all", nil)
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

func TestMarkAllReadSuccess(t *testing.T) {
	notifs := &fakeNotificationRepo{}
	rec := markAllRead(notificationServer(notifs), true, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d (%s)", rec.Code, rec.Body.String())
	}
	if len(notifs.markAllCalls) != 1 || notifs.markAllCalls[0] != "me-id" {
		t.Fatalf("expected MarkAllRead for me-id, got %v", notifs.markAllCalls)
	}
}

func TestMarkAllReadRepeatIdempotent(t *testing.T) {
	srv := notificationServer(&fakeNotificationRepo{})
	_ = markAllRead(srv, true, true)
	rec := markAllRead(srv, true, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("repeat should be 204, got %d", rec.Code)
	}
}

func TestMarkAllReadUnauthenticated(t *testing.T) {
	rec := markAllRead(notificationServer(&fakeNotificationRepo{}), false, true)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMarkAllReadMissingOrigin(t *testing.T) {
	rec := markAllRead(notificationServer(&fakeNotificationRepo{}), true, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestMarkAllReadRepositoryError(t *testing.T) {
	rec := markAllRead(notificationServer(&fakeNotificationRepo{markAllErr: errForTest}), true, true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
