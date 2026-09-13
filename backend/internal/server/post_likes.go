package server

import (
	"context"
	"errors"
	"net/http"

	"together/backend/internal/post"
	"together/backend/internal/user"
)

// resolveAccessiblePost loads the post at {id} and enforces UUID validity,
// existence and visibility for the given viewer. On any failure it writes the
// response (400/404/500) and returns nil.
func (s *Server) resolveAccessiblePost(w http.ResponseWriter, r *http.Request, viewer *user.User) *post.Post {
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, "invalid post id")
		return nil
	}

	p, err := s.posts.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, post.ErrNotFound) {
			writeError(w, http.StatusNotFound, "post not found")
			return nil
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil
	}

	visible, err := s.canViewPost(r.Context(), viewer, p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil
	}
	if !visible {
		writeError(w, http.StatusNotFound, "post not found")
		return nil
	}
	return p
}

func (s *Server) handleLikePost(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	p := s.resolveAccessiblePost(w, r, me)
	if p == nil {
		return
	}
	// Like and its "post_like" notification are created atomically; a duplicate
	// like or a self-like creates no notification.
	if err := s.likeNotify.Like(r.Context(), me.ID, p.ID, p.AuthorID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.writeLikeState(w, r.Context(), p.ID, true)
}

func (s *Server) handleUnlikePost(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	p := s.resolveAccessiblePost(w, r, me)
	if p == nil {
		return
	}
	if err := s.likes.UnlikePost(r.Context(), me.ID, p.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.writeLikeState(w, r.Context(), p.ID, false)
}

func (s *Server) writeLikeState(w http.ResponseWriter, ctx context.Context, postID string, liked bool) {
	count, err := s.likes.CountPostLikes(ctx, postID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"liked": liked, "likesCount": count})
}
