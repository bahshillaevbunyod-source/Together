package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"together/backend/internal/event"
)

func TestValidateEventFields(t *testing.T) {
	start := time.Date(2027, time.January, 2, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	location := "Venue"
	tests := []struct {
		name   string
		mutate func(*string, *string, *string, *string, *string, *time.Time, **time.Time, **string)
		want   bool
	}{
		{"valid in person", func(*string, *string, *string, *string, *string, *time.Time, **time.Time, **string) {}, true},
		{"bad type", func(title, description, timezone, typ, visibility *string, start *time.Time, end **time.Time, url **string) {
			*typ = "bad"
		}, false},
		{"bad visibility", func(title, description, timezone, typ, visibility *string, start *time.Time, end **time.Time, url **string) {
			*visibility = "friends"
		}, false},
		{"bad timezone", func(title, description, timezone, typ, visibility *string, start *time.Time, end **time.Time, url **string) {
			*timezone = "Not/AZone"
		}, false},
		{"end before start", func(title, description, timezone, typ, visibility *string, start *time.Time, end **time.Time, url **string) {
			v := start.Add(-time.Hour)
			*end = &v
		}, false},
		{"dangerous url", func(title, description, timezone, typ, visibility *string, start *time.Time, end **time.Time, url **string) {
			*url = &[]string{"javascript:alert(1)"}[0]
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, description, timezone, typ, visibility := "Meetup", "details", "Asia/Tashkent", event.TypeInPerson, event.VisibilityPublic
			endValue := &end
			urlValue := (*string)(nil)
			name := &location
			tt.mutate(&title, &description, &timezone, &typ, &visibility, &start, &endValue, &urlValue)
			if tt.name == "valid in person" {
				endValue = &end
				name = &location
			} else if tt.name == "dangerous url" {
				typ = event.TypeOnline
				name = nil
			}
			if got := validateEventFields(title, description, timezone, typ, visibility, start, endValue, name, nil, urlValue) == nil; got != tt.want {
				t.Fatalf("valid=%v want %v", got, tt.want)
			}
		})
	}
}

func TestEventsRequireAuthentication(t *testing.T) {
	s := newTestServer(fakePinger{})
	for _, path := range []string{"/api/v1/events", "/api/v1/events/00000000-0000-0000-0000-000000000000", "/api/v1/events/00000000-0000-0000-0000-000000000000/attendees"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			s.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != 401 {
				t.Fatalf("expected 401, got %d", rec.Code)
			}
		})
	}
}

func TestListEventsRejectsInvalidFilters(t *testing.T) {
	srv := discoverServer(&fakeUserRepo{}, mkDiscoverUser("me-id", "me", "", ""))
	long := strings.Repeat("я", maxEventQuery+1)
	for _, q := range []string{"?type=bogus", "?q=" + url.QueryEscape(long)} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events"+q, nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d", q[:10], rec.Code)
		}
	}
}
