package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"together/backend/internal/config"
	"together/backend/internal/follow"
)

func listServer(users *fakeUserRepo, follows *fakeFollowRepo) *http.Server {
	return buildServerWithFollows(config.Config{Env: "test", Port: "8080"}, users, &fakeSessionRepo{}, follows)
}

func getList(srv *http.Server, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func mkListItem(id, username string) follow.ListItem {
	return follow.ListItem{
		ID: id, Username: username, DisplayName: username,
		NativeLanguage: "en", CreatedAt: time.Now(),
	}
}

func decodeList(t *testing.T, rec *httptest.ResponseRecorder) followListResponse {
	t.Helper()
	var resp followListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	return resp
}

func targetUsers() *fakeUserRepo {
	return &fakeUserRepo{usernameUser: mkUser("target-id", "target_user")}
}

func TestFollowersListSuccess(t *testing.T) {
	follows := &fakeFollowRepo{followersList: []follow.ListItem{mkListItem("a", "alice")}}
	rec := getList(listServer(targetUsers(), follows), "/api/v1/users/target_user/followers")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	resp := decodeList(t, rec)
	if len(resp.Items) != 1 || resp.Items[0].Username != "alice" {
		t.Fatalf("unexpected items: %+v", resp.Items)
	}
}

func TestFollowingListSuccess(t *testing.T) {
	follows := &fakeFollowRepo{followingList: []follow.ListItem{mkListItem("b", "bob")}}
	rec := getList(listServer(targetUsers(), follows), "/api/v1/users/target_user/following")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(decodeList(t, rec).Items) != 1 {
		t.Fatal("expected 1 item")
	}
}

func TestFollowListPaginationNextCursor(t *testing.T) {
	// 3 items available, limit 2 -> handler fetches 3, returns 2 + nextCursor.
	follows := &fakeFollowRepo{followersList: []follow.ListItem{
		mkListItem("a", "alice"), mkListItem("b", "bob"), mkListItem("c", "carol"),
	}}
	rec := getList(listServer(targetUsers(), follows), "/api/v1/users/target_user/followers?limit=2")
	resp := decodeList(t, rec)
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.NextCursor == "" {
		t.Fatal("expected a nextCursor")
	}
	// The cursor must decode back to a valid cursor.
	if _, ok := parseCursor(resp.NextCursor); !ok {
		t.Fatal("nextCursor is not decodable")
	}
}

func TestFollowListInvalidCursor(t *testing.T) {
	rec := getList(listServer(targetUsers(), &fakeFollowRepo{}), "/api/v1/users/target_user/followers?cursor=@@@bad")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestFollowListLimitTooLarge(t *testing.T) {
	rec := getList(listServer(targetUsers(), &fakeFollowRepo{}), "/api/v1/users/target_user/followers?limit=100")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestFollowListUnknownUser(t *testing.T) {
	users := &fakeUserRepo{usernameUser: nil} // ErrNotFound
	rec := getList(listServer(users, &fakeFollowRepo{}), "/api/v1/users/ghost/followers")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestFollowListRepositoryError(t *testing.T) {
	follows := &fakeFollowRepo{listErr: errForTest}
	rec := getList(listServer(targetUsers(), follows), "/api/v1/users/target_user/followers")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestFollowListHidesPrivateFields(t *testing.T) {
	follows := &fakeFollowRepo{followersList: []follow.ListItem{mkListItem("a", "alice")}}
	rec := getList(listServer(targetUsers(), follows), "/api/v1/users/target_user/followers")
	body := rec.Body.String()
	for _, forbidden := range []string{"email", "phone", "password"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("list leaked %q: %s", forbidden, body)
		}
	}
}
