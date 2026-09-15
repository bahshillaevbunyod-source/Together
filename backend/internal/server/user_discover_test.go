package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"together/backend/internal/config"
	"together/backend/internal/user"
)

func mkDiscoverUser(id, username, country, lang string) *user.User {
	u := mkUser(id, username)
	if country != "" {
		c := country
		u.CountryCode = &c
	}
	if lang != "" {
		u.NativeLanguage = lang
	}
	return u
}

// discoverServer wires the given viewer (for auth + relevance) and fake repo.
func discoverServer(users *fakeUserRepo, viewer *user.User) *http.Server {
	if users.byID == nil {
		users.byID = map[string]*user.User{}
	}
	users.byID["me-id"] = viewer
	return buildServer(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		users,
		activeSession("me-id"),
	)
}

func doDiscover(srv *http.Server, rawQuery string, withCookie bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/discover"+rawQuery, nil)
	if withCookie {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	}
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func decodeDiscover(t *testing.T, rec *httptest.ResponseRecorder) discoverResponse {
	t.Helper()
	var resp discoverResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v (%s)", err, rec.Body.String())
	}
	return resp
}

func discoverUsernames(resp discoverResponse) []string {
	out := make([]string, len(resp.Items))
	for i, it := range resp.Items {
		out[i] = it.Username
	}
	return out
}

func assertOrder(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order wrong: expected %v, got %v", want, got)
		}
	}
}

func TestDiscoverForYouBoostBreaksTie(t *testing.T) {
	// With equal follower counts, the light country/language boosts nudge
	// relevant users up: same country (+3) > same language (+2) > neither.
	viewer := mkDiscoverUser("me-id", "me_user", "UZ", "uz")
	users := &fakeUserRepo{
		discoverPool: []*user.User{
			mkDiscoverUser("u1", "other", "US", "en"),        // score 10
			mkDiscoverUser("u2", "same_country", "UZ", "en"), // score 13
			mkDiscoverUser("u3", "same_lang", "US", "uz"),    // score 12
		},
		followerCounts: map[string]int64{"u1": 10, "u2": 10, "u3": 10},
	}
	rec := doDiscover(discoverServer(users, viewer), "?mode=for_you", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	assertOrder(t, discoverUsernames(decodeDiscover(t, rec)),
		[]string{"same_country", "same_lang", "other"})
}

func TestDiscoverForYouStrongForeignOutranksWeakLocal(t *testing.T) {
	// The boost is LIGHT, not a hard gate: a popular user from another country
	// outranks a weak same-country/language user.
	viewer := mkDiscoverUser("me-id", "me_user", "UZ", "uz")
	users := &fakeUserRepo{
		discoverPool: []*user.User{
			mkDiscoverUser("u1", "weak_local", "UZ", "uz"),     // score 1+3+2 = 6
			mkDiscoverUser("u2", "strong_foreign", "US", "en"), // score 20
		},
		followerCounts: map[string]int64{"u1": 1, "u2": 20},
	}
	rec := doDiscover(discoverServer(users, viewer), "?mode=for_you", true)
	assertOrder(t, discoverUsernames(decodeDiscover(t, rec)),
		[]string{"strong_foreign", "weak_local"})
}

func TestDiscoverPopularRanking(t *testing.T) {
	viewer := mkDiscoverUser("me-id", "me_user", "UZ", "uz")
	users := &fakeUserRepo{
		discoverPool: []*user.User{
			mkDiscoverUser("a", "alpha", "US", "en"),
			mkDiscoverUser("b", "bravo", "US", "en"),
			mkDiscoverUser("c", "charlie", "US", "en"),
		},
		followerCounts: map[string]int64{"a": 5, "b": 50, "c": 20},
	}
	rec := doDiscover(discoverServer(users, viewer), "?mode=popular", true)
	assertOrder(t, discoverUsernames(decodeDiscover(t, rec)),
		[]string{"bravo", "charlie", "alpha"})
}

func TestDiscoverWorldRanking(t *testing.T) {
	viewer := mkDiscoverUser("me-id", "me_user", "UZ", "uz")
	users := &fakeUserRepo{
		discoverPool: []*user.User{
			mkDiscoverUser("w2", "same_uz", "UZ", "uz"),       // same country -> middle
			mkDiscoverUser("w3", "unknown_country", "", "en"), // unknown -> last
			mkDiscoverUser("w1", "different_us", "US", "en"),  // different known -> first
		},
		followerCounts: map[string]int64{"w1": 1, "w2": 1, "w3": 1},
	}
	rec := doDiscover(discoverServer(users, viewer), "?mode=world", true)
	assertOrder(t, discoverUsernames(decodeDiscover(t, rec)),
		[]string{"different_us", "same_uz", "unknown_country"})
}

func TestDiscoverWorldCountryFilter(t *testing.T) {
	viewer := mkDiscoverUser("me-id", "me_user", "UZ", "uz")
	users := &fakeUserRepo{
		discoverPool: []*user.User{
			mkDiscoverUser("u1", "japan_a", "JP", "ja"),
			mkDiscoverUser("u2", "usa_a", "US", "en"),
			mkDiscoverUser("u3", "japan_b", "JP", "ja"),
		},
	}
	rec := doDiscover(discoverServer(users, viewer), "?mode=world&country=JP", true)
	got := discoverUsernames(decodeDiscover(t, rec))
	if len(got) != 2 {
		t.Fatalf("country filter should yield 2 JP users, got %v", got)
	}
	for _, name := range got {
		if name != "japan_a" && name != "japan_b" {
			t.Fatalf("non-JP user leaked: %v", got)
		}
	}
}

func TestDiscoverExcludesSelf(t *testing.T) {
	viewer := mkDiscoverUser("me-id", "me_user", "UZ", "uz")
	users := &fakeUserRepo{discoverPool: []*user.User{
		mkDiscoverUser("me-id", "me_user", "UZ", "uz"),
		mkDiscoverUser("u1", "someone", "US", "en"),
	}}
	rec := doDiscover(discoverServer(users, viewer), "?mode=popular", true)
	got := discoverUsernames(decodeDiscover(t, rec))
	if len(got) != 1 || got[0] != "someone" {
		t.Fatalf("self should be excluded, got %v", got)
	}
}

func TestDiscoverExcludesFollowed(t *testing.T) {
	viewer := mkDiscoverUser("me-id", "me_user", "UZ", "uz")
	users := &fakeUserRepo{
		discoverPool: []*user.User{
			mkDiscoverUser("u1", "followed_one", "US", "en"),
			mkDiscoverUser("u2", "not_followed", "US", "en"),
		},
		followedByMe: map[string]bool{"u1": true},
	}
	rec := doDiscover(discoverServer(users, viewer), "?mode=popular", true)
	got := discoverUsernames(decodeDiscover(t, rec))
	if len(got) != 1 || got[0] != "not_followed" {
		t.Fatalf("already-followed user should be excluded, got %v", got)
	}
}

func TestDiscoverExcludesBlocked(t *testing.T) {
	viewer := mkDiscoverUser("me-id", "me_user", "UZ", "uz")
	users := &fakeUserRepo{
		discoverPool: []*user.User{
			mkDiscoverUser("u1", "blocked_one", "US", "en"),
			mkDiscoverUser("u2", "visible_one", "US", "en"),
		},
		searchBlocked: map[string]bool{"u1": true},
	}
	rec := doDiscover(discoverServer(users, viewer), "?mode=popular", true)
	got := discoverUsernames(decodeDiscover(t, rec))
	if len(got) != 1 || got[0] != "visible_one" {
		t.Fatalf("blocked user should be excluded, got %v", got)
	}
}

func TestDiscoverKeysetPagination(t *testing.T) {
	viewer := mkDiscoverUser("me-id", "me_user", "UZ", "uz")
	pool := []*user.User{
		mkDiscoverUser("u1", "p1", "US", "en"),
		mkDiscoverUser("u2", "p2", "US", "en"),
		mkDiscoverUser("u3", "p3", "US", "en"),
		mkDiscoverUser("u4", "p4", "US", "en"),
		mkDiscoverUser("u5", "p5", "US", "en"),
	}
	fc := map[string]int64{"u1": 5, "u2": 4, "u3": 3, "u4": 2, "u5": 1}
	users := &fakeUserRepo{discoverPool: pool, followerCounts: fc}
	srv := discoverServer(users, viewer)

	var seen []string
	cursor := ""
	for page := 0; page < 10; page++ {
		q := "?mode=popular&limit=2"
		if cursor != "" {
			q += "&cursor=" + cursor
		}
		resp := decodeDiscover(t, doDiscover(srv, q, true))
		if len(resp.Items) == 0 {
			t.Fatalf("page %d unexpectedly empty", page)
		}
		if len(resp.Items) > 2 {
			t.Fatalf("page %d exceeded limit: %d", page, len(resp.Items))
		}
		seen = append(seen, discoverUsernames(resp)...)
		if resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
	}

	// Ordered, complete, and no duplicates across pages.
	assertOrder(t, seen, []string{"p1", "p2", "p3", "p4", "p5"})
	uniq := map[string]bool{}
	for _, u := range seen {
		if uniq[u] {
			t.Fatalf("duplicate across pages: %s (%v)", u, seen)
		}
		uniq[u] = true
	}
}

func TestDiscoverMalformedCursor(t *testing.T) {
	viewer := mkDiscoverUser("me-id", "me_user", "UZ", "uz")
	// Not valid base64url.
	if rec := doDiscover(discoverServer(&fakeUserRepo{}, viewer), "?mode=popular&cursor=@@@bad", true); rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed base64 cursor: expected 400, got %d", rec.Code)
	}
	// Valid base64url but the payload has no mode/id.
	empty := encodeDiscoverCursor(discoverCursorPayload{})
	if rec := doDiscover(discoverServer(&fakeUserRepo{}, viewer), "?mode=popular&cursor="+empty, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty-payload cursor: expected 400, got %d", rec.Code)
	}
}

func TestDiscoverWorldCursorSameCountry(t *testing.T) {
	viewer := mkDiscoverUser("me-id", "me_user", "UZ", "uz")
	pool := []*user.User{
		mkDiscoverUser("u1", "jp1", "JP", "ja"),
		mkDiscoverUser("u2", "jp2", "JP", "ja"),
		mkDiscoverUser("u3", "jp3", "JP", "ja"),
	}
	fc := map[string]int64{"u1": 3, "u2": 2, "u3": 1}
	srv := discoverServer(&fakeUserRepo{discoverPool: pool, followerCounts: fc}, viewer)

	page1 := decodeDiscover(t, doDiscover(srv, "?mode=world&country=JP&limit=2", true))
	assertOrder(t, discoverUsernames(page1), []string{"jp1", "jp2"})
	if page1.NextCursor == "" {
		t.Fatal("expected a next cursor")
	}
	// Same country filter on the next page must succeed and continue cleanly.
	page2 := decodeDiscover(t, doDiscover(srv,
		"?mode=world&country=JP&limit=2&cursor="+page1.NextCursor, true))
	assertOrder(t, discoverUsernames(page2), []string{"jp3"})
}

func TestDiscoverWorldCursorCountryMismatch(t *testing.T) {
	viewer := mkDiscoverUser("me-id", "me_user", "UZ", "uz")
	// Cursor bound to country=JP, replayed against country=US -> 400.
	jpCursor := encodeDiscoverCursor(discoverCursorPayload{
		Mode: "world", CountryRank: 0, FollowerCount: 2, Country: "JP", ID: "u1",
	})
	rec := doDiscover(discoverServer(&fakeUserRepo{}, viewer),
		"?mode=world&country=US&cursor="+jpCursor, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("changed country filter: expected 400, got %d", rec.Code)
	}
	// Dropping the filter entirely also mismatches the JP-bound cursor.
	rec = doDiscover(discoverServer(&fakeUserRepo{}, viewer),
		"?mode=world&cursor="+jpCursor, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("dropped country filter: expected 400, got %d", rec.Code)
	}
}

func TestDiscoverCursorModeMismatch(t *testing.T) {
	viewer := mkDiscoverUser("me-id", "me_user", "UZ", "uz")
	// A cursor minted for popular must be rejected on a for_you request.
	popularCursor := encodeDiscoverCursor(discoverCursorPayload{Mode: "popular", FollowerCount: 5, ID: "u1"})
	rec := doDiscover(discoverServer(&fakeUserRepo{}, viewer), "?mode=for_you&cursor="+popularCursor, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("cursor/mode mismatch: expected 400, got %d", rec.Code)
	}
}

func TestDiscoverDefaultMode(t *testing.T) {
	viewer := mkDiscoverUser("me-id", "me_user", "UZ", "uz")
	users := &fakeUserRepo{}
	doDiscover(discoverServer(users, viewer), "", true)
	if users.lastDiscover.Mode != user.DiscoverForYou {
		t.Fatalf("default mode should be for_you, got %q", users.lastDiscover.Mode)
	}
	if users.lastDiscover.ViewerCountry != "UZ" || users.lastDiscover.ViewerLanguage != "uz" {
		t.Fatalf("viewer relevance not forwarded: %+v", users.lastDiscover)
	}
}

func TestDiscoverLimitCap(t *testing.T) {
	viewer := mkDiscoverUser("me-id", "me_user", "UZ", "uz")
	users := &fakeUserRepo{}
	doDiscover(discoverServer(users, viewer), "?mode=popular&limit=500", true)
	// Handler caps to 30 and fetches limit+1 to detect the next page.
	if users.lastDiscover.Limit != discoverMaxLimit+1 {
		t.Fatalf("expected repo limit %d, got %d", discoverMaxLimit+1, users.lastDiscover.Limit)
	}
}

func TestDiscoverInvalidMode(t *testing.T) {
	viewer := mkDiscoverUser("me-id", "me_user", "UZ", "uz")
	rec := doDiscover(discoverServer(&fakeUserRepo{}, viewer), "?mode=bogus", true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDiscoverInvalidLimitAndCursor(t *testing.T) {
	viewer := mkDiscoverUser("me-id", "me_user", "UZ", "uz")
	if rec := doDiscover(discoverServer(&fakeUserRepo{}, viewer), "?limit=abc", true); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad limit: expected 400, got %d", rec.Code)
	}
	if rec := doDiscover(discoverServer(&fakeUserRepo{}, viewer), "?limit=0", true); rec.Code != http.StatusBadRequest {
		t.Fatalf("zero limit: expected 400, got %d", rec.Code)
	}
	if rec := doDiscover(discoverServer(&fakeUserRepo{}, viewer), "?cursor=abc", true); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad cursor: expected 400, got %d", rec.Code)
	}
	if rec := doDiscover(discoverServer(&fakeUserRepo{}, viewer), "?cursor=-1", true); rec.Code != http.StatusBadRequest {
		t.Fatalf("negative cursor: expected 400, got %d", rec.Code)
	}
}

func TestDiscoverUnauthenticated(t *testing.T) {
	viewer := mkDiscoverUser("me-id", "me_user", "UZ", "uz")
	rec := doDiscover(discoverServer(&fakeUserRepo{}, viewer), "?mode=popular", false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestDiscoverRepoError(t *testing.T) {
	viewer := mkDiscoverUser("me-id", "me_user", "UZ", "uz")
	users := &fakeUserRepo{discoverErr: errForTest}
	rec := doDiscover(discoverServer(users, viewer), "?mode=popular", true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestDiscoverResponseShape(t *testing.T) {
	viewer := mkDiscoverUser("me-id", "me_user", "UZ", "uz")
	users := &fakeUserRepo{
		discoverPool:   []*user.User{mkDiscoverUser("u1", "alice", "US", "en")},
		followerCounts: map[string]int64{"u1": 42},
	}
	rec := doDiscover(discoverServer(users, viewer), "?mode=popular", true)
	resp := decodeDiscover(t, rec)
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	it := resp.Items[0]
	if it.Username != "alice" || it.FollowerCount != 42 || it.CountryCode == nil || *it.CountryCode != "US" {
		t.Fatalf("unexpected item shape: %+v", it)
	}
}
