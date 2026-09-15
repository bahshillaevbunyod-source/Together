package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"together/backend/internal/config"
	"together/backend/internal/user"
)

func mkSearchUser(id, username, displayName string) *user.User {
	u := mkUser(id, username)
	u.DisplayName = displayName
	return u
}

// searchServer wires the viewer (me-id) for auth plus the given fake repo.
func searchServer(users *fakeUserRepo) *http.Server {
	if users.byID == nil {
		users.byID = map[string]*user.User{}
	}
	users.byID["me-id"] = mkUser("me-id", "me_user")
	return buildServer(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		users,
		activeSession("me-id"),
	)
}

func doSearch(srv *http.Server, rawQuery string, withCookie bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/search"+rawQuery, nil)
	if withCookie {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	}
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func decodeSearch(t *testing.T, rec *httptest.ResponseRecorder) userSearchResponse {
	t.Helper()
	var resp userSearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v (%s)", err, rec.Body.String())
	}
	return resp
}

func usernames(resp userSearchResponse) []string {
	out := make([]string, len(resp.Items))
	for i, it := range resp.Items {
		out[i] = it.Username
	}
	return out
}

func TestSearchByUsername(t *testing.T) {
	users := &fakeUserRepo{searchPool: []*user.User{
		mkSearchUser("u1", "alice_dev", "Alice Cooper"),
		mkSearchUser("u2", "bob_smith", "Bob Smith"),
	}}
	rec := doSearch(searchServer(users), "?q=alice", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	got := usernames(decodeSearch(t, rec))
	if len(got) != 1 || got[0] != "alice_dev" {
		t.Fatalf("expected [alice_dev], got %v", got)
	}
}

func TestSearchByDisplayName(t *testing.T) {
	users := &fakeUserRepo{searchPool: []*user.User{
		mkSearchUser("u1", "xyz123", "Bunyod Bakhshillaev"),
		mkSearchUser("u2", "bob_smith", "Bob Smith"),
	}}
	rec := doSearch(searchServer(users), "?q=bunyod", true)
	got := usernames(decodeSearch(t, rec))
	if len(got) != 1 || got[0] != "xyz123" {
		t.Fatalf("expected [xyz123] via display name, got %v", got)
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	users := &fakeUserRepo{searchPool: []*user.User{
		mkSearchUser("u1", "alice_dev", "Alice Cooper"),
	}}
	rec := doSearch(searchServer(users), "?q=ALICE", true)
	got := usernames(decodeSearch(t, rec))
	if len(got) != 1 || got[0] != "alice_dev" {
		t.Fatalf("case-insensitive match failed, got %v", got)
	}
}

func TestSearchRanking(t *testing.T) {
	users := &fakeUserRepo{searchPool: []*user.User{
		mkSearchUser("u4", "mysammy", "Q Person"), // contains -> rank 3
		mkSearchUser("u3", "zzz1", "Sam Smith"),   // display prefix -> rank 2
		mkSearchUser("u2", "samuel", "Y Person"),  // username prefix -> rank 1
		mkSearchUser("u1", "sam", "X Person"),     // exact username -> rank 0
	}}
	rec := doSearch(searchServer(users), "?q=sam", true)
	got := usernames(decodeSearch(t, rec))
	want := []string{"sam", "samuel", "zzz1", "mysammy"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ranking wrong: expected %v, got %v", want, got)
		}
	}
}

func TestSearchShortQueryReturnsEmpty(t *testing.T) {
	users := &fakeUserRepo{searchPool: []*user.User{mkSearchUser("u1", "alice_dev", "Alice")}}
	rec := doSearch(searchServer(users), "?q=a", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if items := decodeSearch(t, rec).Items; len(items) != 0 {
		t.Fatalf("expected empty items for short query, got %v", items)
	}
	if users.searchCalled {
		t.Fatal("repo should not be queried for a sub-minimum query")
	}
}

func TestSearchEmptyQueryReturnsEmpty(t *testing.T) {
	users := &fakeUserRepo{}
	rec := doSearch(searchServer(users), "", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if items := decodeSearch(t, rec).Items; len(items) != 0 {
		t.Fatalf("expected empty items, got %v", items)
	}
	if users.searchCalled {
		t.Fatal("repo should not be queried for an empty query")
	}
}

func TestSearchTrimsQuery(t *testing.T) {
	users := &fakeUserRepo{searchPool: []*user.User{mkSearchUser("u1", "alice_dev", "Alice")}}
	doSearch(searchServer(users), "?q=%20%20alice%20%20", true) // "  alice  "
	if users.lastSearchQuery != "alice" {
		t.Fatalf("query not trimmed, got %q", users.lastSearchQuery)
	}
}

func TestSearchLimitDefault(t *testing.T) {
	pool := make([]*user.User, 0, 10)
	for i := 0; i < 10; i++ {
		id := string(rune('a' + i))
		pool = append(pool, mkSearchUser("u"+id, "tester_"+id, "Tester "+id))
	}
	users := &fakeUserRepo{searchPool: pool}
	rec := doSearch(searchServer(users), "?q=tester", true)
	if users.lastSearchLimit != 8 {
		t.Fatalf("expected default limit 8, got %d", users.lastSearchLimit)
	}
	if items := decodeSearch(t, rec).Items; len(items) != 8 {
		t.Fatalf("expected 8 items with default limit, got %d", len(items))
	}
}

func TestSearchLimitCap(t *testing.T) {
	users := &fakeUserRepo{searchPool: []*user.User{mkSearchUser("u1", "alice_dev", "Alice")}}
	doSearch(searchServer(users), "?q=alice&limit=50", true)
	if users.lastSearchLimit != 20 {
		t.Fatalf("expected limit capped at 20, got %d", users.lastSearchLimit)
	}
}

func TestSearchInvalidLimit(t *testing.T) {
	users := &fakeUserRepo{}
	if rec := doSearch(searchServer(users), "?q=alice&limit=abc", true); rec.Code != http.StatusBadRequest {
		t.Fatalf("non-numeric limit: expected 400, got %d", rec.Code)
	}
	if rec := doSearch(searchServer(&fakeUserRepo{}), "?q=alice&limit=0", true); rec.Code != http.StatusBadRequest {
		t.Fatalf("zero limit: expected 400, got %d", rec.Code)
	}
}

func TestSearchExcludesBlocked(t *testing.T) {
	users := &fakeUserRepo{
		searchPool: []*user.User{
			mkSearchUser("u1", "alice_dev", "Alice"),
			mkSearchUser("u2", "alice_blocked", "Alice Blocked"),
		},
		searchBlocked: map[string]bool{"u2": true},
	}
	rec := doSearch(searchServer(users), "?q=alice", true)
	got := usernames(decodeSearch(t, rec))
	if len(got) != 1 || got[0] != "alice_dev" {
		t.Fatalf("blocked user should be excluded, got %v", got)
	}
}

func TestSearchExcludesSelf(t *testing.T) {
	users := &fakeUserRepo{searchPool: []*user.User{
		mkSearchUser("me-id", "me_user", "Me Myself"), // the viewer
		mkSearchUser("u1", "me_friend", "Me Friend"),
	}}
	rec := doSearch(searchServer(users), "?q=me_", true)
	got := usernames(decodeSearch(t, rec))
	if len(got) != 1 || got[0] != "me_friend" {
		t.Fatalf("self should be excluded, got %v", got)
	}
}

func TestSearchUnauthenticated(t *testing.T) {
	rec := doSearch(searchServer(&fakeUserRepo{}), "?q=alice", false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestSearchRepoError(t *testing.T) {
	users := &fakeUserRepo{searchErr: errForTest}
	rec := doSearch(searchServer(users), "?q=alice", true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
