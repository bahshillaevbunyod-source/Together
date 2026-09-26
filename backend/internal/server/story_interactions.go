package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"together/backend/internal/conversation"
	"together/backend/internal/story"
)

// Story interactions: likes, the owner-only viewer list, and replies. Every
// mutation first resolves the story through GetActiveVisible, so the existing
// access rule (self-or-accepted-follow, no block either way, 24h window) gates
// likes and replies exactly as it gates viewing — an inaccessible, expired or
// deleted story answers 404 and nothing is written.

const (
	maxViewerListLimit     = 50
	defaultViewerListLimit = 30
)

type storyLikeResponse struct {
	Liked bool `json:"liked"`
}

type storyViewerUserResponse struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"displayName"`
	AvatarURL   *string `json:"avatarUrl"`
}

type storyViewerResponse struct {
	User     storyViewerUserResponse `json:"user"`
	ViewedAt string                  `json:"viewedAt"`
	Liked    bool                    `json:"liked"`
}

type storyViewerListResponse struct {
	Items      []storyViewerResponse `json:"items"`
	Total      int                   `json:"total"`
	NextCursor string                `json:"nextCursor"`
}

type storyReplyRequest struct {
	Content string `json:"content"`
}

type storyReplyResponse struct {
	ConversationID string          `json:"conversationId"`
	Message        messageResponse `json:"message"`
}

// visibleStoryForInteraction resolves an active story the caller may see and
// rejects the caller's own story. It writes the error response itself.
func (s *Server) visibleStoryForInteraction(w http.ResponseWriter, r *http.Request, meID string) (*story.Item, bool) {
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		writeError(w, http.StatusNotFound, "story not found")
		return nil, false
	}
	it, err := s.stories.GetActiveVisible(r.Context(), meID, id)
	if err != nil {
		if errors.Is(err, story.ErrNotFound) {
			writeError(w, http.StatusNotFound, "story not found")
			return nil, false
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil, false
	}
	if it.AuthorID == meID {
		writeError(w, http.StatusBadRequest, "not allowed on your own story")
		return nil, false
	}
	return it, true
}

func (s *Server) handleLikeStory(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	it, ok := s.visibleStoryForInteraction(w, r, me.ID)
	if !ok {
		return
	}
	// A like implies a view: record it first so the owner's viewer list always
	// contains everyone who liked (both inserts are idempotent).
	if _, err := s.stories.RecordView(r.Context(), it.ID, me.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if _, err := s.stories.Like(r.Context(), it.ID, me.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, storyLikeResponse{Liked: true})
}

func (s *Server) handleUnlikeStory(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	it, ok := s.visibleStoryForInteraction(w, r, me.ID)
	if !ok {
		return
	}
	if _, err := s.stories.Unlike(r.Context(), it.ID, me.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, storyLikeResponse{Liked: false})
}

// handleStoryViewers returns the viewer list of the caller's OWN active story.
// Anyone else — including viewers of the story — gets 404, so a story's
// audience can never be enumerated by a non-owner.
func (s *Server) handleStoryViewers(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		writeError(w, http.StatusNotFound, "story not found")
		return
	}
	st, err := s.stories.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, story.ErrNotFound) {
			writeError(w, http.StatusNotFound, "story not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if st.AuthorID != me.ID || time.Since(st.CreatedAt) >= 24*time.Hour {
		writeError(w, http.StatusNotFound, "story not found")
		return
	}

	limit := defaultViewerListLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, ok := parseListLimit(raw)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = min(n, maxViewerListLimit)
	}
	var cur *story.ViewerCursor
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		c, ok := parseStoryCursor(raw)
		if !ok || c == nil {
			writeError(w, http.StatusBadRequest, "invalid cursor")
			return
		}
		cur = &story.ViewerCursor{ViewedAt: c.CreatedAt, UserID: c.ID}
	}

	rows, err := s.stories.ListViewers(r.Context(), id, cur, limit+1)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	total, err := s.stories.CountViewers(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	next := ""
	if len(rows) > limit {
		last := rows[limit-1]
		next = encodeCursor(last.ViewedAt, last.UserID)
		rows = rows[:limit]
	}
	items := make([]storyViewerResponse, 0, len(rows))
	for _, v := range rows {
		items = append(items, storyViewerResponse{
			User:     storyViewerUserResponse{ID: v.UserID, Username: v.Username, DisplayName: v.DisplayName, AvatarURL: v.AvatarURL},
			ViewedAt: v.ViewedAt.Format(time.RFC3339),
			Liked:    v.Liked,
		})
	}
	writeJSON(w, http.StatusOK, storyViewerListResponse{Items: items, Total: total, NextCursor: next})
}

// handleStoryReply sends a reply to a story as a real direct message in the
// existing 1:1 conversation with the author (created on first use, never
// duplicated), then links the message to the story for context.
func (s *Server) handleStoryReply(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "content-type must be application/json")
		return
	}
	it, ok := s.visibleStoryForInteraction(w, r, me.ID)
	if !ok {
		return
	}

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxMessageBodyBytes))
	dec.DisallowUnknownFields()
	var req storyReplyRequest
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Same guard as opening a conversation: a block in either direction
	// forbids it (the story access rule already excludes blocked pairs; this
	// keeps the messaging rule explicit and independent).
	blocked, err := s.blocks.HasBlockBetween(r.Context(), me.ID, it.AuthorID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if blocked {
		writeError(w, http.StatusForbidden, "interaction not allowed")
		return
	}

	conv, err := s.conversations.OpenPrivateConversation(r.Context(), me.ID, it.AuthorID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	m, recipientID, err := s.conversations.CreateMessage(r.Context(), conv.ID, me.ID, req.Content)
	if err != nil {
		switch {
		case errors.Is(err, conversation.ErrEmptyContent), errors.Is(err, conversation.ErrContentTooLong):
			writeError(w, http.StatusBadRequest, "invalid content")
		case errors.Is(err, conversation.ErrBlocked):
			writeError(w, http.StatusForbidden, "interaction not allowed")
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	// The message is committed; a failed context link must not fail the reply.
	storyID := it.ID
	if err := s.stories.RecordReply(r.Context(), m.ID, storyID); err != nil {
		log.Printf("story reply: context link not stored")
	}
	s.resolveCreatedMessageLanguage(r.Context(), m, me.ID)

	resp := messageResponse{
		ID:                       m.ID,
		SenderID:                 m.SenderID,
		Content:                  m.Content,
		CreatedAt:                m.CreatedAt.Format(time.RFC3339),
		UpdatedAt:                m.CreatedAt.Format(time.RFC3339),
		SourceLanguage:           m.SourceLanguage,
		SourceLanguageConfidence: m.SourceLanguageConfidence,
		SourceLanguageResolution: m.SourceLanguageResolution,
		Sender:                   s.messageSenderResponse(r.Context(), m.SenderID),
		StoryReply:               &messageStoryReplyResponse{StoryID: &storyID},
	}
	s.publishMessageCreatedToConversation(r.Context(), conv.ID, me.ID, recipientID, resp)
	writeJSON(w, http.StatusCreated, storyReplyResponse{ConversationID: conv.ID, Message: resp})
}

// attachStoryReplyRefs marks listed messages that are story replies. Errors
// only drop the optional context label; the messages themselves are returned.
func (s *Server) attachStoryReplyRefs(ctx context.Context, items []messageResponse) {
	if s.stories == nil || len(items) == 0 {
		return
	}
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	refs, err := s.stories.ReplyRefs(ctx, ids)
	if err != nil {
		return
	}
	for i := range items {
		if sid, ok := refs[items[i].ID]; ok {
			items[i].StoryReply = &messageStoryReplyResponse{StoryID: sid}
		}
	}
}
