package server

import "net/http"

func (s *Server) handleBlock(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "cannot block yourself")
		return
	}
	if err := s.blocks.Block(r.Context(), me.ID, target.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"blocked": true})
}

func (s *Server) handleUnblock(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	target := s.resolveTarget(w, r)
	if target == nil {
		return
	}
	if err := s.blocks.Unblock(r.Context(), me.ID, target.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"blocked": false})
}
