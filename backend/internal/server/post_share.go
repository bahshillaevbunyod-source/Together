package server

import (
	"errors"
	"net/http"

	"together/backend/internal/post"
)

// handleSharePost creates a provenance-preserving profile share. Only public
// originals are shareable: this keeps a shared feed item from becoming a new
// path to followers-only or private material. The original is never copied.
func (s *Server) handleSharePost(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	original := s.resolveAccessiblePost(w, r, me)
	if original == nil {
		return
	}
	if original.Visibility != post.VisibilityPublic {
		writeError(w, http.StatusNotFound, "post not found")
		return
	}
	created, err := s.posts.CreateRepost(r.Context(), me.ID, original.ID)
	if errors.Is(err, post.ErrAlreadyShared) {
		writeError(w, http.StatusConflict, "post already shared")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	resp, err := s.buildPostResponse(r.Context(), created, me, me)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}
