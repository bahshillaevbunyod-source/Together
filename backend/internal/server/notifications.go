package server

import (
	"net/http"
	"time"

	"together/backend/internal/notification"
)

type notificationActor struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"displayName"`
	AvatarURL   *string `json:"avatarUrl"`
}

type notificationResponse struct {
	ID        string             `json:"id"`
	Type      string             `json:"type"`
	Actor     *notificationActor `json:"actor"`
	PostID    *string            `json:"postId"`
	CommentID *string            `json:"commentId"`
	ReadAt    *string            `json:"readAt"`
	CreatedAt string             `json:"createdAt"`
}

type notificationListResponse struct {
	Items      []notificationResponse `json:"items"`
	NextCursor string                 `json:"nextCursor"`
}

func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
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
	cur, ok := parseNotificationCursor(r.URL.Query().Get("cursor"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid cursor")
		return
	}

	rows, err := s.notifications.List(r.Context(), me.ID, cur, limit+1)
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

	items := make([]notificationResponse, 0, len(rows))
	for _, it := range rows {
		items = append(items, toNotificationResponse(it))
	}

	writeJSON(w, http.StatusOK, notificationListResponse{Items: items, NextCursor: next})
}

func (s *Server) handleMarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, "invalid notification id")
		return
	}

	found, err := s.notifications.MarkRead(r.Context(), me.ID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "notification not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnreadNotificationCount(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	count, err := s.notifications.CountUnread(r.Context(), me.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"unreadCount": count})
}

func (s *Server) handleMarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	if err := s.notifications.MarkAllRead(r.Context(), me.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toNotificationResponse(it notification.ListItem) notificationResponse {
	resp := notificationResponse{
		ID:        it.ID,
		Type:      it.Type,
		PostID:    it.PostID,
		CommentID: it.CommentID,
		CreatedAt: it.CreatedAt.Format(time.RFC3339),
	}
	if it.ActorID != nil {
		resp.Actor = &notificationActor{
			ID:          *it.ActorID,
			Username:    derefString(it.ActorUsername),
			DisplayName: derefString(it.ActorDisplayName),
			AvatarURL:   it.ActorAvatarURL,
		}
	}
	if it.ReadAt != nil {
		s := it.ReadAt.Format(time.RFC3339)
		resp.ReadAt = &s
	}
	return resp
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// parseNotificationCursor decodes a notification cursor. Empty -> nil.
func parseNotificationCursor(raw string) (*notification.Cursor, bool) {
	if raw == "" {
		return nil, true
	}
	t, id, ok := decodeCursor(raw)
	if !ok {
		return nil, false
	}
	return &notification.Cursor{CreatedAt: t, ID: id}, true
}
