package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"together/backend/internal/config"
	"together/backend/internal/followrequest"
	"together/backend/internal/followrequestservice"
	"together/backend/internal/session"
	"together/backend/internal/user"
)

// --- fakes ---------------------------------------------------------------

// fakeFollowRequestRepo is a test double for followrequest.Repository.
type fakeFollowRequestRepo struct {
	created   bool
	exists    bool
	existed   bool // Delete return value
	count     int64
	incoming  []followrequest.ListItem
	createErr error
	existsErr error
	deleteErr error
	listErr   error
	countErr  error

	creates       [][2]string
	deletes       [][2]string
	lastListUser  string
	lastListLimit int
}

func (f *fakeFollowRequestRepo) Create(_ context.Context, requesterID, targetID string) (bool, error) {
	f.creates = append(f.creates, [2]string{requesterID, targetID})
	return f.created, f.createErr
}

func (f *fakeFollowRequestRepo) CreateTx(_ context.Context, _ followrequest.DBTX, requesterID, targetID string) (bool, error) {
	f.creates = append(f.creates, [2]string{requesterID, targetID})
	return f.created, f.createErr
}

func (f *fakeFollowRequestRepo) Exists(_ context.Context, _, _ string) (bool, error) {
	return f.exists, f.existsErr
}

func (f *fakeFollowRequestRepo) Delete(_ context.Context, requesterID, targetID string) (bool, error) {
	f.deletes = append(f.deletes, [2]string{requesterID, targetID})
	return f.existed, f.deleteErr
}

func (f *fakeFollowRequestRepo) DeleteTx(_ context.Context, _ followrequest.DBTX, requesterID, targetID string) (bool, error) {
	f.deletes = append(f.deletes, [2]string{requesterID, targetID})
	return f.existed, f.deleteErr
}

func (f *fakeFollowRequestRepo) CountIncoming(_ context.Context, _ string) (int64, error) {
	return f.count, f.countErr
}

func (f *fakeFollowRequestRepo) ListIncoming(_ context.Context, userID string, _ *followrequest.Cursor, limit int) ([]followrequest.ListItem, error) {
	f.lastListUser = userID
	f.lastListLimit = limit
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.incoming, nil
}

// fakeFollowRequestService is a test double for the followRequestService
// interface (Request + Accept).
type fakeFollowRequestService struct {
	requestCreated bool
	requestErr     error
	acceptErr      error
	requests       [][2]string
	accepts        [][2]string
}

func (f *fakeFollowRequestService) Request(_ context.Context, requesterID, targetID string) (bool, error) {
	f.requests = append(f.requests, [2]string{requesterID, targetID})
	return f.requestCreated, f.requestErr
}

func (f *fakeFollowRequestService) Accept(_ context.Context, requesterID, targetID string) error {
	f.accepts = append(f.accepts, [2]string{requesterID, targetID})
	return f.acceptErr
}

// --- harness -------------------------------------------------------------

func privateUser(id, username string) *user.User {
	u := mkUser(id, username)
	u.IsPrivate = true
	return u
}

// frServer wires a server for follow / follow-request handler tests, exposing
// the fakes the tests assert against.
func frServer(
	target *user.User,
	follows *fakeFollowRepo,
	blocks *fakeBlockRepo,
	notifier *fakeFollowNotifier,
	reqRepo *fakeFollowRequestRepo,
	reqSvc *fakeFollowRequestService,
) *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byIDUser: me, usernameUser: target}
	return New(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users,
		&fakeSessionRepo{active: &session.Session{UserID: "me-id", ExpiresAt: time.Now().Add(time.Hour)}},
		follows, blocks, &fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{},
		&fakeMediaRepo{}, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{},
		&fakePostCreate{}, notifier, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
		reqRepo, reqSvc,
	)
}

func doReq(srv *http.Server, method, path string, withCookie, withOrigin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
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

// --- 1. follow behavior --------------------------------------------------

func TestPublicFollowRemainsImmediate(t *testing.T) {
	notifier := &fakeFollowNotifier{}
	reqSvc := &fakeFollowRequestService{}
	srv := frServer(mkUser("target-id", "target_user"), &fakeFollowRepo{}, &fakeBlockRepo{}, notifier, &fakeFollowRequestRepo{}, reqSvc)

	rec := doReq(srv, http.MethodPost, "/api/v1/users/target_user/follow", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"following":true`) || !strings.Contains(body, `"requested":false`) {
		t.Fatalf("unexpected body: %s", body)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("public follow must create the follows edge via notifier, got %d calls", len(notifier.calls))
	}
	if len(reqSvc.requests) != 0 {
		t.Fatal("public follow must NOT create a follow request")
	}
}

func TestPrivateFollowCreatesRequestNotEdge(t *testing.T) {
	notifier := &fakeFollowNotifier{}
	follows := &fakeFollowRepo{is: false}
	reqSvc := &fakeFollowRequestService{requestCreated: true}
	srv := frServer(privateUser("target-id", "target_user"), follows, &fakeBlockRepo{}, notifier, &fakeFollowRequestRepo{}, reqSvc)

	rec := doReq(srv, http.MethodPost, "/api/v1/users/target_user/follow", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"following":false`) || !strings.Contains(body, `"requested":true`) {
		t.Fatalf("unexpected body: %s", body)
	}
	// A request was created for (me -> target)...
	if len(reqSvc.requests) != 1 || reqSvc.requests[0] != [2]string{"me-id", "target-id"} {
		t.Fatalf("expected Request(me-id,target-id), got %v", reqSvc.requests)
	}
	// ...and NO follows edge was created.
	if len(notifier.calls) != 0 || len(follows.followed) != 0 {
		t.Fatal("private follow must NOT create a follows edge")
	}
}

func TestPrivateFollowWhenAlreadyFollowingIsIdempotent(t *testing.T) {
	notifier := &fakeFollowNotifier{}
	follows := &fakeFollowRepo{is: true} // already following (followed before going private)
	reqSvc := &fakeFollowRequestService{}
	srv := frServer(privateUser("target-id", "target_user"), follows, &fakeBlockRepo{}, notifier, &fakeFollowRequestRepo{}, reqSvc)

	rec := doReq(srv, http.MethodPost, "/api/v1/users/target_user/follow", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"following":true`) || !strings.Contains(body, `"requested":false`) {
		t.Fatalf("unexpected body: %s", body)
	}
	if len(reqSvc.requests) != 0 {
		t.Fatal("already-following private follow must not create a request")
	}
}

func TestPrivateFollowDuplicateRequestIdempotent(t *testing.T) {
	reqSvc := &fakeFollowRequestService{requestCreated: false} // second time: no new row
	srv := frServer(privateUser("target-id", "target_user"), &fakeFollowRepo{is: false}, &fakeBlockRepo{}, &fakeFollowNotifier{}, &fakeFollowRequestRepo{}, reqSvc)

	rec := doReq(srv, http.MethodPost, "/api/v1/users/target_user/follow", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("duplicate private follow should be 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"requested":true`) {
		t.Fatalf("duplicate private follow should still report requested:true, got %s", rec.Body.String())
	}
}

func TestPrivateFollowBlockedRejected(t *testing.T) {
	reqSvc := &fakeFollowRequestService{}
	blocks := &fakeBlockRepo{hasBetween: true}
	srv := frServer(privateUser("target-id", "target_user"), &fakeFollowRepo{}, blocks, &fakeFollowNotifier{}, &fakeFollowRequestRepo{}, reqSvc)

	rec := doReq(srv, http.MethodPost, "/api/v1/users/target_user/follow", true, true)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("blocked private follow should be 403, got %d", rec.Code)
	}
	if len(reqSvc.requests) != 0 {
		t.Fatal("a blocked user must not create a follow request")
	}
}

// --- 2. cancel -----------------------------------------------------------

func TestCancelFollowRequest(t *testing.T) {
	reqRepo := &fakeFollowRequestRepo{existed: true}
	srv := frServer(privateUser("target-id", "target_user"), &fakeFollowRepo{}, &fakeBlockRepo{}, &fakeFollowNotifier{}, reqRepo, &fakeFollowRequestService{})

	rec := doReq(srv, http.MethodDelete, "/api/v1/users/target_user/follow-request", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"requested":false`) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
	// Only the caller's own request (me -> target) is deleted.
	if len(reqRepo.deletes) != 1 || reqRepo.deletes[0] != [2]string{"me-id", "target-id"} {
		t.Fatalf("expected Delete(me-id,target-id), got %v", reqRepo.deletes)
	}
}

func TestCancelFollowRequestIdempotent(t *testing.T) {
	reqRepo := &fakeFollowRequestRepo{existed: false} // nothing to cancel
	srv := frServer(privateUser("target-id", "target_user"), &fakeFollowRepo{}, &fakeBlockRepo{}, &fakeFollowNotifier{}, reqRepo, &fakeFollowRequestService{})

	rec := doReq(srv, http.MethodDelete, "/api/v1/users/target_user/follow-request", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("cancelling a missing request should still be 200, got %d", rec.Code)
	}
}

// --- 3. incoming requests ------------------------------------------------

func TestListIncomingFollowRequests(t *testing.T) {
	now := time.Now()
	reqRepo := &fakeFollowRequestRepo{
		incoming: []followrequest.ListItem{
			{ID: "a", Username: "alice", DisplayName: "Alice", CreatedAt: now},
			{ID: "b", Username: "bob", DisplayName: "Bob", CreatedAt: now.Add(-time.Minute)},
		},
	}
	srv := frServer(nil, &fakeFollowRepo{}, &fakeBlockRepo{}, &fakeFollowNotifier{}, reqRepo, &fakeFollowRequestService{})

	// limit=1 with 2 rows returned -> one item plus a next cursor.
	rec := doReq(srv, http.MethodGet, "/api/v1/follow-requests?limit=1", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"username":"alice"`) || !strings.Contains(body, `"createdAt"`) {
		t.Fatalf("unexpected body: %s", body)
	}
	if strings.Contains(body, `"nextCursor":""`) {
		t.Fatalf("expected a non-empty next cursor when more rows exist: %s", body)
	}
	if reqRepo.lastListUser != "me-id" {
		t.Fatalf("inbox must list the authenticated user's incoming requests, got %q", reqRepo.lastListUser)
	}
}

func TestListIncomingRequestsRequiresAuth(t *testing.T) {
	srv := frServer(nil, &fakeFollowRepo{}, &fakeBlockRepo{}, &fakeFollowNotifier{}, &fakeFollowRequestRepo{}, &fakeFollowRequestService{})
	rec := doReq(srv, http.MethodGet, "/api/v1/follow-requests", false, true)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// --- 4. accept / decline -------------------------------------------------

func TestAcceptFollowRequest(t *testing.T) {
	reqSvc := &fakeFollowRequestService{}
	// {username} in the path is the REQUESTER; the authenticated user is target.
	srv := frServer(mkUser("requester-id", "requester_user"), &fakeFollowRepo{}, &fakeBlockRepo{}, &fakeFollowNotifier{}, &fakeFollowRequestRepo{}, reqSvc)

	rec := doReq(srv, http.MethodPost, "/api/v1/follow-requests/requester_user/accept", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"following":true`) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
	if len(reqSvc.accepts) != 1 || reqSvc.accepts[0] != [2]string{"requester-id", "me-id"} {
		t.Fatalf("expected Accept(requester-id, me-id), got %v", reqSvc.accepts)
	}
}

func TestAcceptMissingRequestReturns404(t *testing.T) {
	reqSvc := &fakeFollowRequestService{acceptErr: followrequestservice.ErrNoRequest}
	srv := frServer(mkUser("requester-id", "requester_user"), &fakeFollowRepo{}, &fakeBlockRepo{}, &fakeFollowNotifier{}, &fakeFollowRequestRepo{}, reqSvc)

	rec := doReq(srv, http.MethodPost, "/api/v1/follow-requests/requester_user/accept", true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("accepting a missing request should be 404, got %d", rec.Code)
	}
}

func TestDeclineFollowRequest(t *testing.T) {
	reqRepo := &fakeFollowRequestRepo{existed: true}
	srv := frServer(mkUser("requester-id", "requester_user"), &fakeFollowRepo{}, &fakeBlockRepo{}, &fakeFollowNotifier{}, reqRepo, &fakeFollowRequestService{})

	rec := doReq(srv, http.MethodPost, "/api/v1/follow-requests/requester_user/decline", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	// Deletes the (requester -> me) request; never creates a follows edge.
	if len(reqRepo.deletes) != 1 || reqRepo.deletes[0] != [2]string{"requester-id", "me-id"} {
		t.Fatalf("expected Delete(requester-id, me-id), got %v", reqRepo.deletes)
	}
}

func TestDeclineMissingRequestIdempotent(t *testing.T) {
	reqRepo := &fakeFollowRequestRepo{existed: false}
	srv := frServer(mkUser("requester-id", "requester_user"), &fakeFollowRepo{}, &fakeBlockRepo{}, &fakeFollowNotifier{}, reqRepo, &fakeFollowRequestService{})

	rec := doReq(srv, http.MethodPost, "/api/v1/follow-requests/requester_user/decline", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("declining a missing request should still be 200, got %d", rec.Code)
	}
}

// --- 5. profile fields ---------------------------------------------------

func profileReq(srv *http.Server, username string, withCookie bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+username, nil)
	if withCookie {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	}
	req.Header.Set("Origin", testOrigin)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func TestPublicProfileExposesIsPrivateAndFollowRequested(t *testing.T) {
	follows := &fakeFollowRepo{is: false}
	reqRepo := &fakeFollowRequestRepo{exists: true} // viewer has a pending request
	srv := frServer(privateUser("target-id", "target_user"), follows, &fakeBlockRepo{}, &fakeFollowNotifier{}, reqRepo, &fakeFollowRequestService{})

	rec := profileReq(srv, "target_user", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"isPrivate":true`) {
		t.Fatalf("expected isPrivate:true, got %s", body)
	}
	if !strings.Contains(body, `"followRequested":true`) {
		t.Fatalf("expected followRequested:true, got %s", body)
	}
	// isFollowing semantics unchanged: still false here.
	if !strings.Contains(body, `"isFollowing":false`) {
		t.Fatalf("expected isFollowing:false, got %s", body)
	}
}

func TestPublicProfileFollowRequestedFalseWhenNoRequest(t *testing.T) {
	follows := &fakeFollowRepo{is: true} // following
	reqRepo := &fakeFollowRequestRepo{exists: false}
	srv := frServer(mkUser("target-id", "target_user"), follows, &fakeBlockRepo{}, &fakeFollowNotifier{}, reqRepo, &fakeFollowRequestService{})

	rec := profileReq(srv, "target_user", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"isPrivate":false`) {
		t.Fatalf("expected isPrivate:false, got %s", body)
	}
	if !strings.Contains(body, `"followRequested":false`) {
		t.Fatalf("expected followRequested:false, got %s", body)
	}
	if !strings.Contains(body, `"isFollowing":true`) {
		t.Fatalf("expected isFollowing:true (unchanged semantics), got %s", body)
	}
}
