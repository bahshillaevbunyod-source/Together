package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"together/backend/internal/user"
)

func doWorldCountries(srv *http.Server, withCookie bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/world/countries", nil)
	if withCookie {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	}
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func TestWorldCountriesRequiresAuth(t *testing.T) {
	srv := discoverServer(&fakeUserRepo{}, mkDiscoverUser("me-id", "me", "UZ", "uz"))
	if rec := doWorldCountries(srv, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestWorldCountriesCountsOnlyDiscoverablePeople(t *testing.T) {
	users := &fakeUserRepo{
		discoverPool: []*user.User{
			mkDiscoverUser("me-id", "me", "UZ", "uz"),       // self: excluded
			mkDiscoverUser("a-id", "a", "UZ", "uz"),         // counted
			mkDiscoverUser("b-id", "b", "DE", "de"),         // counted
			mkDiscoverUser("c-id", "c", "DE", "de"),         // counted
			mkDiscoverUser("blocked-id", "blk", "DE", "de"), // blocked: excluded
			mkDiscoverUser("followed-id", "fol", "FR", ""),  // followed: excluded
			mkDiscoverUser("nocountry-id", "none", "", ""),  // no country: excluded
		},
		searchBlocked: map[string]bool{"blocked-id": true},
		followedByMe:  map[string]bool{"followed-id": true},
	}
	srv := discoverServer(users, mkDiscoverUser("me-id", "me", "UZ", "uz"))
	rec := doWorldCountries(srv, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Items []worldCountryItem `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	want := []worldCountryItem{{CountryCode: "DE", People: 2}, {CountryCode: "UZ", People: 1}}
	if !reflect.DeepEqual(resp.Items, want) {
		t.Fatalf("got %+v want %+v", resp.Items, want)
	}
}

func TestWorldCountriesEmptyIsArray(t *testing.T) {
	srv := discoverServer(&fakeUserRepo{}, mkDiscoverUser("me-id", "me", "", ""))
	rec := doWorldCountries(srv, true)
	if rec.Code != http.StatusOK || rec.Body.String() != "{\"items\":[]}\n" {
		t.Fatalf("expected empty items array, got %d %q", rec.Code, rec.Body.String())
	}
}

func TestWorldCountriesRepoError(t *testing.T) {
	users := &fakeUserRepo{discoverErr: errors.New("boom")}
	srv := discoverServer(users, mkDiscoverUser("me-id", "me", "", ""))
	if rec := doWorldCountries(srv, true); rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
