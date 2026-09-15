package server

import (
	"net/http"
	"strconv"
	"strings"
)

const (
	searchMinQueryLen  = 2
	searchDefaultLimit = 8
	searchMaxLimit     = 20
)

type userSearchItem struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"displayName"`
	AvatarURL   *string `json:"avatarUrl"`
}

type userSearchResponse struct {
	Items []userSearchItem `json:"items"`
}

// handleUserSearch serves GET /api/v1/users/search?q=&limit=. Auth required.
// Queries shorter than searchMinQueryLen return an empty list (not an error) so
// the client can call it freely as the user types. limit defaults to 8 and is
// hard-capped at 20. Block filtering and ranking happen in the repository.
func (s *Server) handleUserSearch(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < searchMinQueryLen {
		writeJSON(w, http.StatusOK, userSearchResponse{Items: []userSearchItem{}})
		return
	}

	limit := searchDefaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = n
	}
	if limit > searchMaxLimit {
		limit = searchMaxLimit
	}

	results, err := s.users.SearchUsers(r.Context(), me.ID, q, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	items := make([]userSearchItem, 0, len(results))
	for _, u := range results {
		items = append(items, userSearchItem{
			ID:          u.ID,
			Username:    u.Username,
			DisplayName: u.DisplayName,
			AvatarURL:   u.AvatarURL,
		})
	}
	writeJSON(w, http.StatusOK, userSearchResponse{Items: items})
}
