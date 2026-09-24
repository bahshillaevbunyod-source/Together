package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"together/backend/internal/call"
	"together/backend/internal/conversation"
	"together/backend/internal/user"
)

const maxCallBodyBytes = 16 << 10

type callPeerResponse struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"displayName"`
	AvatarURL   *string `json:"avatarUrl"`
}

func (s *Server) handleCallConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"iceServers": []any{}})
}

type createCallRequest struct {
	ConversationID string `json:"conversationId"`
	Type           string `json:"type"`
}

type directPeerLookup interface {
	DirectPeer(context.Context, string, string) (string, string, error)
}

func (s *Server) handleCreateCall(w http.ResponseWriter, r *http.Request) {
	me, _ := CurrentUser(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxCallBodyBytes)
	var req createCallRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil || !uuidPattern.MatchString(req.ConversationID) || (req.Type != "voice" && req.Type != "video") {
		writeError(w, http.StatusBadRequest, "invalid call request")
		return
	}
	lookup, ok := s.conversations.(directPeerLookup)
	if !ok {
		writeError(w, http.StatusNotImplemented, "calls unavailable")
		return
	}
	kind, calleeID, err := lookup.DirectPeer(r.Context(), req.ConversationID, me.ID)
	if errors.Is(err, conversation.ErrNotParticipant) {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if kind != "direct" || calleeID == me.ID {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	blocked, err := s.blocks.HasBlockBetween(r.Context(), me.ID, calleeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if blocked {
		writeError(w, http.StatusForbidden, "interaction not allowed")
		return
	}
	e, err := s.calls.Create(req.ConversationID, me.ID, calleeID, call.Type(req.Type))
	if errors.Is(err, call.ErrBusy) {
		writeError(w, http.StatusConflict, "participant busy")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.calls.ArmTimeout(e.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"callId": e.ID, "iceServers": []any{}})
}

func callPeer(u *user.User) callPeerResponse {
	return callPeerResponse{ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, AvatarURL: u.AvatarURL}
}

func (s *Server) relayCall(userID, typ string, data any) {
	b, err := json.Marshal(map[string]any{"type": typ, "data": data})
	if err == nil {
		s.hub.SendToUser(userID, b)
	}
}

func validStringField(m map[string]json.RawMessage, name string, max int) (string, bool) {
	var v string
	b, ok := m[name]
	if !ok || json.Unmarshal(b, &v) != nil || len(v) == 0 || len(v) > max {
		return "", false
	}
	return v, true
}
