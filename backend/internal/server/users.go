package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"together/backend/internal/user"
)

// publicUserResponse is the public view of a user. It never exposes email,
// phone or password_hash.
type publicUserResponse struct {
	ID             string  `json:"id"`
	Username       string  `json:"username"`
	DisplayName    string  `json:"displayName"`
	AvatarURL      *string `json:"avatarUrl"`
	Bio            *string `json:"bio"`
	CountryCode    *string `json:"countryCode"`
	City           *string `json:"city"`
	NativeLanguage string  `json:"nativeLanguage"`
	CreatedAt      string  `json:"createdAt"`
	FollowersCount int64   `json:"followersCount"`
	FollowingCount int64   `json:"followingCount"`
	IsFollowing    bool    `json:"isFollowing"`
	IsSelf         bool    `json:"isSelf"`
}

func toPublicUserResponse(u *user.User) publicUserResponse {
	return publicUserResponse{
		ID:             u.ID,
		Username:       u.Username,
		DisplayName:    u.DisplayName,
		AvatarURL:      u.AvatarURL,
		Bio:            u.Bio,
		CountryCode:    u.CountryCode,
		City:           u.City,
		NativeLanguage: u.NativeLanguage,
		CreatedAt:      u.CreatedAt.Format(time.RFC3339),
	}
}

// handlePublicProfile serves a public user profile by username. No auth needed.
func (s *Server) handlePublicProfile(w http.ResponseWriter, r *http.Request) {
	username := strings.ToLower(strings.TrimSpace(r.PathValue("username")))
	if username == "" {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	u, err := s.users.GetByUsername(r.Context(), username)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := toPublicUserResponse(u)

	followers, err := s.follows.CountFollowers(r.Context(), u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	following, err := s.follows.CountFollowing(r.Context(), u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	resp.FollowersCount = followers
	resp.FollowingCount = following

	// Optional auth: relationship state only when a viewer is authenticated.
	if viewer := s.optionalUser(r); viewer != nil {
		resp.IsSelf = viewer.ID == u.ID
		if !resp.IsSelf {
			isFollowing, err := s.follows.IsFollowing(r.Context(), viewer.ID, u.ID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			resp.IsFollowing = isFollowing
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
