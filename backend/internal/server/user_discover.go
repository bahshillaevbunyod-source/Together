package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"together/backend/internal/user"
)

const (
	discoverDefaultLimit = 12
	discoverMaxLimit     = 30
)

type discoverItem struct {
	ID             string  `json:"id"`
	Username       string  `json:"username"`
	DisplayName    string  `json:"displayName"`
	AvatarURL      *string `json:"avatarUrl"`
	CountryCode    *string `json:"countryCode"`
	City           *string `json:"city"`
	NativeLanguage string  `json:"nativeLanguage"`
	FollowerCount  int64   `json:"followerCount"`
}

type discoverResponse struct {
	Items      []discoverItem `json:"items"`
	NextCursor string         `json:"nextCursor"`
}

// discoverCursorPayload is the opaque, mode-aware keyset cursor. It carries the
// mode plus the sort-key values of the last row on the previous page. Only the
// fields relevant to the mode are used; ID is always present as the tie-break.
type discoverCursorPayload struct {
	Mode          string    `json:"m"`
	Score         int64     `json:"s,omitempty"`
	FollowerCount int64     `json:"fc,omitempty"`
	CountryRank   int       `json:"r,omitempty"`
	CreatedAt     time.Time `json:"t,omitempty"`
	Country       string    `json:"c,omitempty"` // world mode: bound country filter ("" = no filter)
	ID            string    `json:"id"`
}

func encodeDiscoverCursor(p discoverCursorPayload) string {
	b, _ := json.Marshal(p)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeDiscoverCursor(s string) (discoverCursorPayload, bool) {
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return discoverCursorPayload{}, false
	}
	var p discoverCursorPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return discoverCursorPayload{}, false
	}
	if p.Mode == "" || !uuidPattern.MatchString(p.ID) {
		return discoverCursorPayload{}, false
	}
	return p, true
}

// handleUserDiscover serves GET /api/v1/users/discover?mode=&country=&limit=&cursor=.
// Auth required. mode is one of for_you (default), world, popular. cursor is an
// opaque, mode-aware keyset token; a malformed cursor or one whose mode differs
// from the request mode is a 400. Results exclude self, blocked relationships
// (both directions), and already-followed users.
func (s *Server) handleUserDiscover(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = user.DiscoverForYou
	}
	switch mode {
	case user.DiscoverForYou, user.DiscoverWorld, user.DiscoverPopular:
	default:
		writeError(w, http.StatusBadRequest, "invalid mode")
		return
	}

	limit := discoverDefaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = n
	}
	if limit > discoverMaxLimit {
		limit = discoverMaxLimit
	}

	// Normalized country filter (world mode). Bound into the cursor so a page
	// token can't be replayed against a different filter.
	country := strings.TrimSpace(r.URL.Query().Get("country"))

	var after *user.DiscoverCursor
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		payload, ok := decodeDiscoverCursor(raw)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid cursor")
			return
		}
		if payload.Mode != mode {
			writeError(w, http.StatusBadRequest, "cursor does not match mode")
			return
		}
		if mode == user.DiscoverWorld && payload.Country != country {
			writeError(w, http.StatusBadRequest, "cursor does not match country filter")
			return
		}
		after = &user.DiscoverCursor{
			Score:         payload.Score,
			FollowerCount: payload.FollowerCount,
			CountryRank:   payload.CountryRank,
			CreatedAt:     payload.CreatedAt,
			ID:            payload.ID,
		}
	}

	viewerCountry := ""
	if me.CountryCode != nil {
		viewerCountry = *me.CountryCode
	}

	// Fetch one extra row to detect whether another page exists.
	results, err := s.users.DiscoverUsers(r.Context(), user.DiscoverParams{
		ViewerID:       me.ID,
		Mode:           mode,
		Country:        country,
		ViewerCountry:  viewerCountry,
		ViewerLanguage: me.NativeLanguage,
		After:          after,
		Limit:          limit + 1,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	next := ""
	if len(results) > limit {
		results = results[:limit]
		last := results[len(results)-1]
		payload := discoverCursorPayload{Mode: mode, ID: last.ID}
		switch mode {
		case user.DiscoverPopular:
			payload.FollowerCount = last.FollowerCount
		case user.DiscoverWorld:
			payload.CountryRank = last.CountryRank
			payload.FollowerCount = last.FollowerCount
			payload.Country = country
		default: // for_you
			payload.Score = last.Score
			payload.CreatedAt = last.CreatedAt
		}
		next = encodeDiscoverCursor(payload)
	}

	items := make([]discoverItem, 0, len(results))
	for _, u := range results {
		items = append(items, discoverItem{
			ID:             u.ID,
			Username:       u.Username,
			DisplayName:    u.DisplayName,
			AvatarURL:      u.AvatarURL,
			CountryCode:    u.CountryCode,
			City:           u.City,
			NativeLanguage: u.NativeLanguage,
			FollowerCount:  u.FollowerCount,
		})
	}
	writeJSON(w, http.StatusOK, discoverResponse{Items: items, NextCursor: next})
}
