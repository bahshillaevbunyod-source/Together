package server

import (
	"errors"
	"net/http"
	"time"

	"together/backend/internal/followrequest"
	"together/backend/internal/followrequestservice"
)

// requestListItem is one incoming follow request in a private account's inbox:
// the requester's public-safe fields plus when the request was made.
type requestListItem struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"displayName"`
	AvatarURL   *string `json:"avatarUrl"`
	CreatedAt   string  `json:"createdAt"`
}

type requestListResponse struct {
	Items      []requestListItem `json:"items"`
	NextCursor string            `json:"nextCursor"`
}

// handleCancelFollowRequest lets the authenticated requester withdraw their own
// pending request to {username}. Idempotent: cancelling a missing request still
// succeeds. It only ever deletes the caller's own request row and never touches
// the follows graph.
func (s *Server) handleCancelFollowRequest(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	target := s.resolveTarget(w, r)
	if target == nil {
		return
	}
	if _, err := s.followRequests.Delete(r.Context(), me.ID, target.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, followResponse{Following: false, Requested: false})
}

// handleListFollowRequests returns the authenticated user's incoming pending
// follow requests, newest-first with keyset pagination. Requesters in a block
// relationship with the viewer are filtered out in SQL.
func (s *Server) handleListFollowRequests(w http.ResponseWriter, r *http.Request) {
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
	cur, ok := parseRequestCursor(r.URL.Query().Get("cursor"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid cursor")
		return
	}

	// Fetch one extra row to detect whether another page exists.
	rows, err := s.followRequests.ListIncoming(r.Context(), me.ID, cur, limit+1)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	next := ""
	if len(rows) > limit {
		last := rows[limit-1]
		next = encodeCursor(last.CreatedAt, last.ID)
		rows = rows[:limit]
	}

	items := make([]requestListItem, 0, len(rows))
	for _, it := range rows {
		items = append(items, requestListItem{
			ID:          it.ID,
			Username:    it.Username,
			DisplayName: it.DisplayName,
			AvatarURL:   it.AvatarURL,
			CreatedAt:   it.CreatedAt.Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, requestListResponse{Items: items, NextCursor: next})
}

// handleAcceptFollowRequest approves an incoming request. The path {username} is
// the REQUESTER; the authenticated user is the target/owner, so only the owner
// can accept requests addressed to them. Accept is atomic (delete request +
// insert follows edge + follow notification). A missing request yields 404.
func (s *Server) handleAcceptFollowRequest(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	requester := s.resolveTarget(w, r)
	if requester == nil {
		return
	}

	err := s.followReq.Accept(r.Context(), requester.ID, me.ID)
	if err != nil {
		if errors.Is(err, followrequestservice.ErrNoRequest) {
			writeError(w, http.StatusNotFound, "request not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, followResponse{Following: true, Requested: false})
}

// handleDeclineFollowRequest rejects an incoming request. The path {username} is
// the REQUESTER; the authenticated user is the target/owner. It deletes only the
// request row and never creates a follows edge. Idempotent: declining a missing
// request still succeeds.
func (s *Server) handleDeclineFollowRequest(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	requester := s.resolveTarget(w, r)
	if requester == nil {
		return
	}
	if _, err := s.followRequests.Delete(r.Context(), requester.ID, me.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, followResponse{Following: false, Requested: false})
}

// parseRequestCursor decodes an incoming-request list cursor. Empty -> nil
// (first page). Invalid -> ok=false.
func parseRequestCursor(raw string) (*followrequest.Cursor, bool) {
	if raw == "" {
		return nil, true
	}
	t, id, ok := decodeCursor(raw)
	if !ok || !uuidPattern.MatchString(id) {
		return nil, false
	}
	return &followrequest.Cursor{CreatedAt: t, UserID: id}, true
}
