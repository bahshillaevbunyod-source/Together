package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"together/backend/internal/conversation"
	"together/backend/internal/translation"
	"together/backend/internal/user"
)

const maxMessageBodyBytes = 1 << 20 // 1 MiB

type createMessageRequest struct {
	Content string `json:"content"`
}

type messageResponse struct {
	ID        string `json:"id"`
	SenderID  string `json:"senderId"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`

	// Translation fields are null unless the viewer opted in and a translation
	// succeeded. Content always holds the original, untranslated text.
	TranslatedContent *string `json:"translatedContent"`
	SourceLanguage    *string `json:"sourceLanguage"`
	TargetLanguage    *string `json:"targetLanguage"`
}

type messageListResponse struct {
	Items      []messageResponse `json:"items"`
	NextCursor string            `json:"nextCursor"`
}

type conversationOtherUser struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"displayName"`
	AvatarURL   *string `json:"avatarUrl"`
}

type conversationLastMessage struct {
	ID        string `json:"id"`
	SenderID  string `json:"senderId"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
}

type conversationResponse struct {
	ID          string                   `json:"id"`
	OtherUser   conversationOtherUser    `json:"otherUser"`
	LastMessage *conversationLastMessage `json:"lastMessage"`
	UnreadCount int64                    `json:"unreadCount"`
	UpdatedAt   string                   `json:"updatedAt"`
}

type conversationListResponse struct {
	Items      []conversationResponse `json:"items"`
	NextCursor string                 `json:"nextCursor"`
}

type openConversationRequest struct {
	Username string `json:"username"`
}

// handleOpenConversation opens (or returns the existing) private conversation
// between the caller and the target user identified by username.
func (s *Server) handleOpenConversation(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "content-type must be application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxMessageBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req openConversationRequest
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	username := strings.ToLower(strings.TrimSpace(req.Username))
	if username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}

	target, err := s.users.GetByUsername(r.Context(), username)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if target.ID == me.ID {
		writeError(w, http.StatusBadRequest, "cannot open a conversation with yourself")
		return
	}

	// A block in either direction forbids opening a conversation. The response
	// never reveals who blocked whom.
	blocked, err := s.blocks.HasBlockBetween(r.Context(), me.ID, target.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if blocked {
		writeError(w, http.StatusForbidden, "interaction not allowed")
		return
	}

	conv, err := s.conversations.OpenPrivateConversation(r.Context(), me.ID, target.ID)
	if err != nil {
		if errors.Is(err, conversation.ErrSameUser) {
			writeError(w, http.StatusBadRequest, "cannot open a conversation with yourself")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Frontend-friendly shape (matches GET /conversations items). Idempotent:
	// an existing pair returns that conversation, never a duplicate.
	writeJSON(w, http.StatusOK, conversationResponse{
		ID: conv.ID,
		OtherUser: conversationOtherUser{
			ID:          target.ID,
			Username:    target.Username,
			DisplayName: target.DisplayName,
			AvatarURL:   target.AvatarURL,
		},
		LastMessage: nil,
		UnreadCount: 0,
		UpdatedAt:   conv.UpdatedAt.Format(time.RFC3339),
	})
}

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
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
	cur, ok := parseConversationCursor(r.URL.Query().Get("cursor"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid cursor")
		return
	}

	rows, err := s.conversations.ListConversations(r.Context(), me.ID, cur, limit+1)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	next := ""
	if len(rows) > limit {
		last := rows[limit-1]
		next = encodeCursor(last.UpdatedAt, last.ID)
		rows = rows[:limit]
	}

	items := make([]conversationResponse, 0, len(rows))
	for _, it := range rows {
		resp := conversationResponse{
			ID: it.ID,
			OtherUser: conversationOtherUser{
				ID:          it.OtherID,
				Username:    it.OtherUsername,
				DisplayName: it.OtherDisplayName,
				AvatarURL:   it.OtherAvatarURL,
			},
			UnreadCount: it.UnreadCount,
			UpdatedAt:   it.UpdatedAt.Format(time.RFC3339),
		}
		if it.LastMessageID != nil {
			resp.LastMessage = &conversationLastMessage{
				ID:        *it.LastMessageID,
				SenderID:  derefString(it.LastMessageSenderID),
				Content:   derefString(it.LastMessageContent),
				CreatedAt: it.LastMessageCreatedAt.Format(time.RFC3339),
			}
		}
		items = append(items, resp)
	}

	writeJSON(w, http.StatusOK, conversationListResponse{Items: items, NextCursor: next})
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}

	limit, ok := parseListLimit(r.URL.Query().Get("limit"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	cur, ok := parseMessageCursor(r.URL.Query().Get("cursor"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid cursor")
		return
	}

	rows, err := s.conversations.ListMessages(r.Context(), id, me.ID, cur, limit+1)
	if err != nil {
		// Non-members must not learn the conversation exists.
		if errors.Is(err, conversation.ErrNotParticipant) {
			writeError(w, http.StatusNotFound, "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	next := ""
	if len(rows) > limit {
		last := rows[limit-1]
		next = encodeCursor(last.CreatedAt, last.ID)
		rows = rows[:limit]
	}

	// Translate only when the viewer opted in and has a target language.
	targetLang := ""
	if me.AutoTranslateEnabled && me.PreferredLanguage != nil {
		targetLang = *me.PreferredLanguage
	}

	items := make([]messageResponse, 0, len(rows))
	for _, m := range rows {
		resp := messageResponse{
			ID:        m.ID,
			SenderID:  m.SenderID,
			Content:   m.Content, // always the original
			CreatedAt: m.CreatedAt.Format(time.RFC3339),
		}
		s.applyTranslation(r.Context(), &resp, m.Content, targetLang)
		items = append(items, resp)
	}

	writeJSON(w, http.StatusOK, messageListResponse{Items: items, NextCursor: next})
}

// applyTranslation fills the translation fields of resp when targetLang is set.
// A translation failure is swallowed: the original content is preserved and the
// response still succeeds.
func (s *Server) applyTranslation(ctx context.Context, resp *messageResponse, original, targetLang string) {
	if targetLang == "" {
		return
	}
	res, err := s.translator.Translate(ctx, translation.Request{Text: original, TargetLang: targetLang})
	if err != nil {
		return // never fail the response over a translation error
	}
	resp.TranslatedContent = &res.TranslatedText
	resp.SourceLanguage = &res.SourceLang
	resp.TargetLanguage = &res.TargetLang
}

func (s *Server) handleMarkConversationRead(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}

	found, err := s.conversations.MarkConversationRead(r.Context(), id, me.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !found {
		// Non-members must not learn the conversation exists.
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCreateMessage(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}

	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "content-type must be application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxMessageBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req createMessageRequest
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Content validation (trim / empty / length) and membership live in the
	// repository; sender is always the authenticated user, and the recipient is
	// resolved server-side (never from the client).
	m, recipientID, err := s.conversations.CreateMessage(r.Context(), id, me.ID, req.Content)
	if err != nil {
		switch {
		case errors.Is(err, conversation.ErrEmptyContent), errors.Is(err, conversation.ErrContentTooLong):
			writeError(w, http.StatusBadRequest, "invalid content")
		case errors.Is(err, conversation.ErrNotParticipant):
			// Non-members must not learn the conversation exists.
			writeError(w, http.StatusNotFound, "conversation not found")
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	resp := messageResponse{
		ID:        m.ID,
		SenderID:  m.SenderID,
		Content:   m.Content,
		CreatedAt: m.CreatedAt.Format(time.RFC3339),
	}

	// The DB is the source of truth; realtime delivery happens only after a
	// successful commit and must never fail the request. The hub send is
	// non-blocking, so a slow or disconnected recipient cannot stall us. Publish
	// with the untranslated response so the recipient event is translated for
	// the recipient, not the sender.
	s.publishMessageCreated(r.Context(), id, recipientID, resp)

	// Translate the HTTP response for the CURRENT sender/viewer's preferences.
	if me.AutoTranslateEnabled && me.PreferredLanguage != nil {
		s.applyTranslation(r.Context(), &resp, resp.Content, *me.PreferredLanguage)
	}

	writeJSON(w, http.StatusCreated, resp)
}

// realtimeEvent is the envelope pushed to WebSocket clients.
type realtimeEvent struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// messageCreatedData is the "message.created" payload: the message fields plus
// the conversation id so the client can route it. Embedding messageResponse
// flattens its JSON fields alongside conversationId.
type messageCreatedData struct {
	ConversationID string `json:"conversationId"`
	messageResponse
}

// publishMessageCreated sends a "message.created" event to the recipient only.
// The event is translated for the recipient when they opted in; the original
// content is always preserved and no translation (or lookup) failure blocks
// delivery.
func (s *Server) publishMessageCreated(ctx context.Context, conversationID, recipientID string, msg messageResponse) {
	// Translate using the RECIPIENT's preferences, loaded server-side.
	if recipient, err := s.users.GetByID(ctx, recipientID); err == nil &&
		recipient.AutoTranslateEnabled && recipient.PreferredLanguage != nil {
		s.applyTranslation(ctx, &msg, msg.Content, *recipient.PreferredLanguage)
	}

	data := messageCreatedData{ConversationID: conversationID, messageResponse: msg}
	payload, err := json.Marshal(realtimeEvent{Type: "message.created", Data: data})
	if err != nil {
		return // never fail the request over a serialization problem
	}
	s.hub.SendToUser(recipientID, payload)
}

// parseMessageCursor decodes a message cursor. Empty -> nil (first page).
func parseMessageCursor(raw string) (*conversation.MessageCursor, bool) {
	if raw == "" {
		return nil, true
	}
	t, id, ok := decodeCursor(raw)
	if !ok {
		return nil, false
	}
	return &conversation.MessageCursor{CreatedAt: t, ID: id}, true
}

// parseConversationCursor decodes a conversation-list cursor. Empty -> nil.
func parseConversationCursor(raw string) (*conversation.ConversationCursor, bool) {
	if raw == "" {
		return nil, true
	}
	t, id, ok := decodeCursor(raw)
	if !ok {
		return nil, false
	}
	return &conversation.ConversationCursor{UpdatedAt: t, ID: id}, true
}
