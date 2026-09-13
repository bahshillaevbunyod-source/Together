package server

import (
	"errors"
	"net/http"
	"strings"

	"together/backend/internal/user"
)

// resolveTarget loads the follow target by username and returns it, or writes
// the appropriate error response (404 / 500) and returns nil.
func (s *Server) resolveTarget(w http.ResponseWriter, r *http.Request) *user.User {
	username := strings.ToLower(strings.TrimSpace(r.PathValue("username")))
	if username == "" {
		writeError(w, http.StatusNotFound, "user not found")
		return nil
	}
	target, err := s.users.GetByUsername(r.Context(), username)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return nil
	}
	return target
}

func (s *Server) handleFollow(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	target := s.resolveTarget(w, r)
	if target == nil {
		return
	}
	if target.ID == me.ID {
		writeError(w, http.StatusBadRequest, "cannot follow yourself")
		return
	}

	// A block in either direction forbids following. The response never
	// reveals who blocked whom.
	blocked, err := s.blocks.HasBlockBetween(r.Context(), me.ID, target.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if blocked {
		writeError(w, http.StatusForbidden, "interaction not allowed")
		return
	}

	// Follow and its "follow" notification are created atomically; a duplicate
	// follow creates no duplicate notification.
	if err := s.followNotify.Follow(r.Context(), me.ID, target.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"following": true})
}

func (s *Server) handleUnfollow(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	target := s.resolveTarget(w, r)
	if target == nil {
		return
	}
	if err := s.follows.Unfollow(r.Context(), me.ID, target.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"following": false})
}
