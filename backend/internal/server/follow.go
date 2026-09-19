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

	// Private target: never insert a follows edge here. Create a pending request
	// instead (unless already following), so restricted access is unaffected.
	if target.IsPrivate {
		// If already following (e.g. followed before the account went private),
		// report that state idempotently — no request is created.
		following, err := s.follows.IsFollowing(r.Context(), me.ID, target.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if following {
			writeJSON(w, http.StatusOK, followResponse{Following: true, Requested: false})
			return
		}

		// Create (idempotently) the pending request and, only when newly
		// created, its follow_request notification — atomically.
		if _, err := s.followReq.Request(r.Context(), me.ID, target.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, followResponse{Following: false, Requested: true})
		return
	}

	// Public target: unchanged immediate follow. Follow and its "follow"
	// notification are created atomically; a duplicate follow creates no
	// duplicate notification.
	if err := s.followNotify.Follow(r.Context(), me.ID, target.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, followResponse{Following: true, Requested: false})
}

// followResponse is the shape of follow / follow-request mutations. Requested is
// true only when a pending request exists (or was just created) for a private
// target; Following reflects the accepted `follows` relationship.
type followResponse struct {
	Following bool `json:"following"`
	Requested bool `json:"requested"`
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
