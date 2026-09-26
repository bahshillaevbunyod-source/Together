package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"together/backend/internal/community"
	"together/backend/internal/conversation"
	"together/backend/internal/translation"
	"together/backend/internal/user"
)

const maxMessageBodyBytes = 1 << 20 // 1 MiB

type createMessageRequest struct {
	Content    string                          `json:"content"`
	Attachment *createMessageAttachmentRequest `json:"attachment"`
}

type createMessageAttachmentRequest struct {
	StorageKey string `json:"storageKey"`
	Filename   string `json:"filename"`
}
type messageAttachmentResponse struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	Type      string `json:"type"`
	MimeType  string `json:"mimeType"`
	SizeBytes int64  `json:"sizeBytes"`
	URL       string `json:"url"`
}

type muteConversationRequest struct {
	Muted bool `json:"muted"`
}

type messageResponse struct {
	ID         string                     `json:"id"`
	SenderID   string                     `json:"senderId"`
	Content    string                     `json:"content"`
	CreatedAt  string                     `json:"createdAt"`
	UpdatedAt  string                     `json:"updatedAt"`
	DeletedAt  *string                    `json:"deletedAt"`
	Attachment *messageAttachmentResponse `json:"attachment"`
	Sender     *messageSenderResponse     `json:"sender,omitempty"`
	// StoryReply is set when the message was sent as a reply to a story.
	StoryReply *messageStoryReplyResponse `json:"storyReply,omitempty"`

	// Translation fields are null unless the viewer opted in and a translation
	// succeeded. Content always holds the original, untranslated text.
	TranslatedContent        *string  `json:"translatedContent"`
	SourceLanguage           *string  `json:"sourceLanguage"`
	SourceLanguageConfidence *float64 `json:"sourceLanguageConfidence"`
	SourceLanguageResolution *string  `json:"sourceLanguageResolution"`
	TargetLanguage           *string  `json:"targetLanguage"`
}

// messageStoryReplyResponse marks a story reply. StoryID is null once the
// story was deleted (the label stays; the link does not).
type messageStoryReplyResponse struct {
	StoryID *string `json:"storyId"`
}

type messageSenderResponse struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"displayName"`
	AvatarURL   *string `json:"avatarUrl"`
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
	Muted       bool                     `json:"muted"`
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
			Muted:       it.Muted,
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
	targetLang := translationTarget(me)

	items := make([]messageResponse, 0, len(rows))
	for _, m := range rows {
		resp := messageResponse{
			ID:                       m.ID,
			SenderID:                 m.SenderID,
			Content:                  m.Content, // always the original
			CreatedAt:                m.CreatedAt.Format(time.RFC3339),
			UpdatedAt:                m.UpdatedAt.Format(time.RFC3339),
			SourceLanguage:           m.SourceLanguage,
			SourceLanguageConfidence: m.SourceLanguageConfidence,
			SourceLanguageResolution: m.SourceLanguageResolution,
			Attachment:               s.messageAttachmentResponse(r.Context(), m.Attachment),
			Sender:                   s.messageSenderResponse(r.Context(), m.SenderID),
		}
		s.applyTranslation(r.Context(), &resp, m.Content, targetLang)
		items = append(items, resp)
	}
	s.attachStoryReplyRefs(r.Context(), items)

	writeJSON(w, http.StatusOK, messageListResponse{Items: items, NextCursor: next})
}

func (s *Server) messageAttachmentResponse(ctx context.Context, a *conversation.Attachment) *messageAttachmentResponse {
	if a == nil {
		return nil
	}
	return &messageAttachmentResponse{ID: a.ID, Filename: a.Filename, Type: a.Type, MimeType: a.MimeType, SizeBytes: a.SizeBytes, URL: s.mediaURLForKey(ctx, a.StorageKey)}
}

func (s *Server) messageSenderResponse(ctx context.Context, id string) *messageSenderResponse {
	u, err := s.users.GetByID(ctx, id)
	if err != nil {
		return nil
	}
	return &messageSenderResponse{ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, AvatarURL: u.AvatarURL}
}

func safeAttachmentFilename(raw string) string {
	name := strings.TrimSpace(filepath.Base(raw))
	if name == "." || name == "" {
		return "attachment"
	}
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	if len([]rune(name)) > 255 {
		name = string([]rune(name)[:255])
	}
	return name
}

type messageSearchResponse struct {
	Items []messageSearchItem `json:"items"`
}
type messageSearchItem struct {
	ConversationID   string `json:"conversationId"`
	OtherUsername    string `json:"otherUsername"`
	OtherDisplayName string `json:"otherDisplayName"`
	messageResponse
}

func translationTarget(me *user.User) string {
	if me.AutoTranslateEnabled && me.PreferredLanguage != nil {
		return *me.PreferredLanguage
	}
	return ""
}

// handleSearchMessages only searches rows joined to the caller's participant
// record, so a query can never reveal another user's private conversation.
func (s *Server) handleSearchMessages(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" || len([]rune(query)) > 200 {
		writeError(w, http.StatusBadRequest, "invalid query")
		return
	}
	limit, ok := parseListLimit(r.URL.Query().Get("limit"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	rows, err := s.conversations.SearchMessages(r.Context(), me.ID, query, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	items := make([]messageSearchItem, 0, len(rows))
	for _, m := range rows {
		resp := messageResponse{ID: m.ID, SenderID: m.SenderID, Content: m.Content, CreatedAt: m.CreatedAt.Format(time.RFC3339), UpdatedAt: m.UpdatedAt.Format(time.RFC3339), SourceLanguage: m.SourceLanguage, SourceLanguageConfidence: m.SourceLanguageConfidence, SourceLanguageResolution: m.SourceLanguageResolution}
		s.applyTranslation(r.Context(), &resp, m.Content, translationTarget(me))
		items = append(items, messageSearchItem{ConversationID: m.ConversationID, OtherUsername: m.OtherUsername, OtherDisplayName: m.OtherDisplayName, messageResponse: resp})
	}
	writeJSON(w, http.StatusOK, messageSearchResponse{Items: items})
}

// applyTranslation fills the translation fields of resp when targetLang is set.
// A translation failure is swallowed: the original content is preserved and the
// response still succeeds.
func (s *Server) applyTranslation(ctx context.Context, resp *messageResponse, original, targetLang string) {
	if targetLang == "" {
		return
	}
	res, err := s.translator.Translate(ctx, translation.Request{
		Text:       original,
		SourceLang: persistedSourceLanguage(resp),
		TargetLang: targetLang,
	})
	if err != nil {
		return // never fail the response over a translation error
	}
	resp.TranslatedContent = &res.TranslatedText
	if resp.SourceLanguage == nil {
		resp.SourceLanguage = &res.SourceLang
	}
	resp.TargetLanguage = &res.TargetLang
}

func persistedSourceLanguage(resp *messageResponse) string {
	if resp == nil || resp.SourceLanguage == nil || resp.SourceLanguageResolution == nil {
		return ""
	}
	resolution := *resp.SourceLanguageResolution
	if resolution != string(translation.ResolutionCurrent) && resolution != string(translation.ResolutionContext) {
		return ""
	}
	return *resp.SourceLanguage
}

func (s *Server) resolveCreatedMessageLanguage(ctx context.Context, m *conversation.Message, senderID string) {
	if s.languageResolver == nil || m == nil || strings.TrimSpace(m.Content) == "" {
		return
	}

	contextText, err := s.conversations.RecentSenderContext(ctx, m.ConversationID, senderID, m.ID)
	if err != nil {
		log.Printf("source language context lookup unavailable")
		return
	}
	resolution, err := s.languageResolver.Resolve(ctx, m.Content, contextText)
	if err != nil {
		log.Printf("source language detection unavailable")
		return
	}

	var language *string
	var confidence *float64
	if resolution.LanguageCode != "" {
		code := resolution.LanguageCode
		language = &code
		value := resolution.Confidence
		confidence = &value
	}
	resolutionSource := string(resolution.Source)
	if err := s.conversations.SetMessageLanguageMetadata(ctx, m.ID, senderID, language, confidence, resolutionSource); err != nil {
		log.Printf("source language metadata persistence unavailable")
		return
	}

	m.SourceLanguage = language
	m.SourceLanguageConfidence = confidence
	m.SourceLanguageResolution = &resolutionSource
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
	if s.communities != nil {
		if kind, role, authErr := s.communities.Authorize(r.Context(), id, me.ID); authErr == nil && kind == community.Channel && role == community.RoleMember {
			writeError(w, http.StatusForbidden, "channel is read-only for members")
			return
		}
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
	var m *conversation.Message
	var recipientID string
	var err error
	if req.Attachment != nil {
		confirmed, confirmErr := s.validateUploadedObjectWithLimits(r.Context(), me.ID, req.Attachment.StorageKey, &messageMediaLimits, "uploads", "private")
		if confirmErr != nil {
			writeError(w, http.StatusBadRequest, "invalid attachment")
			return
		}
		m, recipientID, err = s.conversations.CreateMessageWithAttachment(r.Context(), id, me.ID, req.Content, conversation.Attachment{StorageKey: confirmed.StorageKey, Filename: safeAttachmentFilename(req.Attachment.Filename), Type: confirmed.Type, MimeType: confirmed.MimeType, SizeBytes: confirmed.SizeBytes, CreatedAt: time.Now()})
	} else {
		m, recipientID, err = s.conversations.CreateMessage(r.Context(), id, me.ID, req.Content)
	}
	if err != nil {
		switch {
		case errors.Is(err, conversation.ErrEmptyContent), errors.Is(err, conversation.ErrContentTooLong):
			writeError(w, http.StatusBadRequest, "invalid content")
		case errors.Is(err, conversation.ErrNotParticipant):
			// Non-members must not learn the conversation exists.
			writeError(w, http.StatusNotFound, "conversation not found")
		case errors.Is(err, conversation.ErrBlocked):
			writeError(w, http.StatusForbidden, "interaction not allowed")
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	s.resolveCreatedMessageLanguage(r.Context(), m, me.ID)

	resp := messageResponse{
		ID:                       m.ID,
		SenderID:                 m.SenderID,
		Content:                  m.Content,
		CreatedAt:                m.CreatedAt.Format(time.RFC3339),
		SourceLanguage:           m.SourceLanguage,
		SourceLanguageConfidence: m.SourceLanguageConfidence,
		SourceLanguageResolution: m.SourceLanguageResolution,
		Attachment:               s.messageAttachmentResponse(r.Context(), m.Attachment),
		Sender:                   s.messageSenderResponse(r.Context(), m.SenderID),
	}

	// The DB is the source of truth; realtime delivery happens only after a
	// successful commit and must never fail the request. The hub send is
	// non-blocking, so a slow or disconnected recipient cannot stall us. Publish
	// with the untranslated response so the recipient event is translated for
	// the recipient, not the sender.
	s.publishMessageCreatedToConversation(r.Context(), id, me.ID, recipientID, resp)

	// Translate the HTTP response for the CURRENT sender/viewer's preferences.
	if me.AutoTranslateEnabled && me.PreferredLanguage != nil {
		s.applyTranslation(r.Context(), &resp, resp.Content, *me.PreferredLanguage)
	}

	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleUpdateMessage(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	messageID := r.PathValue("id")
	if !uuidPattern.MatchString(messageID) {
		writeError(w, http.StatusBadRequest, "invalid message id")
		return
	}
	if s.communities != nil {
		if err := s.communities.AuthorizeMessage(r.Context(), messageID, me.ID); err != nil {
			if errors.Is(err, community.ErrNotMember) {
				writeError(w, http.StatusNotFound, "message not found")
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "content-type must be application/json")
		return
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxMessageBodyBytes))
	dec.DisallowUnknownFields()
	var req createMessageRequest
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	m, recipientID, err := s.conversations.UpdateMessage(r.Context(), messageID, me.ID, req.Content)
	if err != nil {
		switch {
		case errors.Is(err, conversation.ErrEmptyContent), errors.Is(err, conversation.ErrContentTooLong):
			writeError(w, http.StatusBadRequest, "invalid content")
		case errors.Is(err, conversation.ErrMessageNotFound):
			writeError(w, http.StatusNotFound, "message not found")
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	resp := messageResponse{ID: m.ID, SenderID: m.SenderID, Content: m.Content, CreatedAt: m.CreatedAt.Format(time.RFC3339), UpdatedAt: m.UpdatedAt.Format(time.RFC3339), Attachment: s.messageAttachmentResponse(r.Context(), m.Attachment), Sender: s.messageSenderResponse(r.Context(), m.SenderID)}
	s.publishMessageUpdatedToConversation(r.Context(), m.ConversationID, me.ID, recipientID, resp)
	s.applyTranslation(r.Context(), &resp, resp.Content, translationTarget(me))
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	messageID := r.PathValue("id")
	if !uuidPattern.MatchString(messageID) {
		writeError(w, http.StatusBadRequest, "invalid message id")
		return
	}
	if s.communities != nil {
		if err := s.communities.AuthorizeMessage(r.Context(), messageID, me.ID); err != nil {
			if errors.Is(err, community.ErrNotMember) {
				writeError(w, http.StatusNotFound, "message not found")
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
	}
	conversationID, recipientID, deletedAt, err := s.conversations.DeleteMessage(r.Context(), messageID, me.ID)
	if err != nil {
		if errors.Is(err, conversation.ErrMessageNotFound) {
			writeError(w, http.StatusNotFound, "message not found")
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	s.publishMessageDeletedToConversation(r.Context(), conversationID, messageID, me.ID, recipientID, deletedAt)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSetConversationMuted(w http.ResponseWriter, r *http.Request) {
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
	var req muteConversationRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxMessageBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	found, err := s.conversations.SetConversationMuted(r.Context(), id, me.ID, req.Muted)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"muted": req.Muted})
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

type messageDeletedData struct {
	ConversationID string `json:"conversationId"`
	ID             string `json:"id"`
	DeletedAt      string `json:"deletedAt"`
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

func (s *Server) communityRecipients(ctx context.Context, conversationID, senderID, fallback string) []string {
	if s.communities == nil {
		return []string{fallback}
	}
	ids, err := s.communities.MemberIDs(ctx, conversationID)
	if err != nil || len(ids) == 0 {
		return []string{fallback}
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != senderID {
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return []string{fallback}
	}
	return out
}

func (s *Server) publishMessageCreatedToConversation(ctx context.Context, conversationID, senderID, fallback string, msg messageResponse) {
	for _, id := range s.communityRecipients(ctx, conversationID, senderID, fallback) {
		s.publishMessageCreated(ctx, conversationID, id, msg)
	}
}

func (s *Server) publishMessageUpdated(ctx context.Context, conversationID, senderID, recipientID string, msg messageResponse) {
	for _, userID := range []string{senderID, recipientID} {
		out := msg
		if viewer, err := s.users.GetByID(ctx, userID); err == nil {
			s.applyTranslation(ctx, &out, out.Content, translationTarget(viewer))
		}
		payload, err := json.Marshal(realtimeEvent{Type: "message.updated", Data: messageCreatedData{ConversationID: conversationID, messageResponse: out}})
		if err == nil {
			s.hub.SendToUser(userID, payload)
		}
	}
}

func (s *Server) publishMessageUpdatedToConversation(ctx context.Context, conversationID, senderID, fallback string, msg messageResponse) {
	for _, id := range append([]string{senderID}, s.communityRecipients(ctx, conversationID, senderID, fallback)...) {
		s.publishMessageUpdated(ctx, conversationID, senderID, id, msg)
	}
}

func (s *Server) publishMessageDeleted(conversationID, messageID, senderID, recipientID string, deletedAt time.Time) {
	payload, err := json.Marshal(realtimeEvent{Type: "message.deleted", Data: messageDeletedData{ConversationID: conversationID, ID: messageID, DeletedAt: deletedAt.Format(time.RFC3339)}})
	if err != nil {
		return
	}
	s.hub.SendToUser(senderID, payload)
	s.hub.SendToUser(recipientID, payload)
}

func (s *Server) publishMessageDeletedToConversation(ctx context.Context, conversationID, messageID, senderID, fallback string, deletedAt time.Time) {
	for _, id := range append([]string{senderID}, s.communityRecipients(ctx, conversationID, senderID, fallback)...) {
		s.publishMessageDeleted(conversationID, messageID, senderID, id, deletedAt)
	}
}

// parseMessageCursor decodes a message cursor. Empty -> nil (first page).
func parseMessageCursor(raw string) (*conversation.MessageCursor, bool) {
	if raw == "" {
		return nil, true
	}
	t, id, ok := decodeCursor(raw)
	if !ok || !uuidPattern.MatchString(id) {
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
	if !ok || !uuidPattern.MatchString(id) {
		return nil, false
	}
	return &conversation.ConversationCursor{UpdatedAt: t, ID: id}, true
}
