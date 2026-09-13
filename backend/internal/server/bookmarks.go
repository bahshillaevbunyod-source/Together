package server

import "net/http"

func (s *Server) handleSavePost(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	// Same UUID / existence / visibility / block check as read/like.
	p := s.resolveAccessiblePost(w, r, me)
	if p == nil {
		return
	}
	if err := s.bookmarks.Save(r.Context(), me.ID, p.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnsavePost(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	p := s.resolveAccessiblePost(w, r, me)
	if p == nil {
		return
	}
	if err := s.bookmarks.Remove(r.Context(), me.ID, p.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
