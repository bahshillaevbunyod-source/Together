package server

import "net/http"

type blockedUserResponse struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"displayName"`
	AvatarURL   *string `json:"avatarUrl"`
}

func (s *Server) handleListBlocked(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	limit, ok := parseListLimit(r.URL.Query().Get("limit"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	items, err := s.blocks.List(r.Context(), me.ID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	response := make([]blockedUserResponse, 0, len(items))
	for _, item := range items {
		response = append(response, blockedUserResponse{ID: item.ID, Username: item.Username, DisplayName: item.DisplayName, AvatarURL: item.AvatarURL})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": response})
}

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
