package server

import (
	"context"
	"net/http"

	"together/backend/internal/follow"
)

const (
	defaultListLimit = 20
	maxListLimit     = 50
)

type followListItem struct {
	ID             string  `json:"id"`
	Username       string  `json:"username"`
	DisplayName    string  `json:"displayName"`
	AvatarURL      *string `json:"avatarUrl"`
	CountryCode    *string `json:"countryCode"`
	City           *string `json:"city"`
	NativeLanguage string  `json:"nativeLanguage"`
}

type followListResponse struct {
	Items      []followListItem `json:"items"`
	NextCursor string           `json:"nextCursor"`
}

type listFunc func(ctx context.Context, userID string, cur *follow.Cursor, limit int) ([]follow.ListItem, error)

func (s *Server) handleFollowersList(w http.ResponseWriter, r *http.Request) {
	s.serveFollowList(w, r, s.follows.ListFollowers)
}

func (s *Server) handleFollowingList(w http.ResponseWriter, r *http.Request) {
	s.serveFollowList(w, r, s.follows.ListFollowing)
}

func (s *Server) serveFollowList(w http.ResponseWriter, r *http.Request, list listFunc) {
	target := s.resolveTarget(w, r)
	if target == nil {
		return
	}

	limit, ok := parseListLimit(r.URL.Query().Get("limit"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	cur, ok := parseCursor(r.URL.Query().Get("cursor"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid cursor")
		return
	}

	// Fetch one extra row to detect whether another page exists.
	rows, err := list(r.Context(), target.ID, cur, limit+1)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	next := ""
	if len(rows) > limit {
		last := rows[limit-1]
		next = encodeCursor(last.CreatedAt, last.ID)
		rows = rows[:limit]
	}

	items := make([]followListItem, 0, len(rows))
	for _, it := range rows {
		items = append(items, followListItem{
			ID:             it.ID,
			Username:       it.Username,
			DisplayName:    it.DisplayName,
			AvatarURL:      it.AvatarURL,
			CountryCode:    it.CountryCode,
			City:           it.City,
			NativeLanguage: it.NativeLanguage,
		})
	}

	writeJSON(w, http.StatusOK, followListResponse{Items: items, NextCursor: next})
}

// parseCursor decodes a follow-list cursor. Empty -> nil (first page).
// Invalid -> ok=false.
func parseCursor(raw string) (*follow.Cursor, bool) {
	if raw == "" {
		return nil, true
	}
	t, id, ok := decodeCursor(raw)
	if !ok {
		return nil, false
	}
	return &follow.Cursor{CreatedAt: t, UserID: id}, true
}
