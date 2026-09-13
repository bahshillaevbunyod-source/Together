package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"together/backend/internal/comment"
	"together/backend/internal/translation"
	"together/backend/internal/user"
)

const (
	maxCommentBodyBytes    = 1 << 20 // 1 MiB
	maxCommentContentRunes = 2000
)

type createCommentRequest struct {
	Content string `json:"content"`
}

type commentAuthorResponse struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"displayName"`
	AvatarURL   *string `json:"avatarUrl"`
}

type commentResponse struct {
	ID        string                `json:"id"`
	Author    commentAuthorResponse `json:"author"`
	Content   string                `json:"content"`
	CreatedAt string                `json:"createdAt"`
	UpdatedAt string                `json:"updatedAt"`

	// Translation fields are null unless the viewer opted in and a translation
	// succeeded. Content always holds the original, untranslated text.
	TranslatedContent *string `json:"translatedContent"`
	SourceLanguage    *string `json:"sourceLanguage"`
	TargetLanguage    *string `json:"targetLanguage"`
}

// applyCommentTranslation fills the translation fields of resp using the
// viewer's preferences. No-op for anonymous / opted-out / no preferred language
// / empty content. A translation failure is swallowed so the response still
// succeeds with the original content.
func (s *Server) applyCommentTranslation(ctx context.Context, resp *commentResponse, viewer *user.User) {
	if viewer == nil || !viewer.AutoTranslateEnabled || viewer.PreferredLanguage == nil {
		return
	}
	if resp.Content == "" {
		return
	}
	res, err := s.translator.Translate(ctx, translation.Request{Text: resp.Content, TargetLang: *viewer.PreferredLanguage})
	if err != nil {
		return
	}
	resp.TranslatedContent = &res.TranslatedText
	resp.SourceLanguage = &res.SourceLang
	resp.TargetLanguage = &res.TargetLang
}

type commentListResponse struct {
	Items      []commentResponse `json:"items"`
	NextCursor string            `json:"nextCursor"`
}

func (s *Server) handleCreateComment(w http.ResponseWriter, r *http.Request) {
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

	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "content-type must be application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxCommentBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req createCommentRequest
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	if utf8.RuneCountInString(content) > maxCommentContentRunes {
		writeError(w, http.StatusBadRequest, "content is too long")
		return
	}

	// author_id always comes from the authenticated user. The comment and its
	// "post_comment" notification are created atomically; a self-comment
	// creates no notification.
	c, err := s.commentNotify.CreateComment(r.Context(), comment.CreateInput{
		PostID:   p.ID,
		AuthorID: me.ID,
		Content:  content,
	}, p.AuthorID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := commentResponse{
		ID: c.ID,
		Author: commentAuthorResponse{
			ID:          me.ID,
			Username:    me.Username,
			DisplayName: me.DisplayName,
			AvatarURL:   me.AvatarURL,
		},
		Content:   c.Content,
		CreatedAt: c.CreatedAt.Format(time.RFC3339),
		UpdatedAt: c.UpdatedAt.Format(time.RFC3339),
	}
	s.applyCommentTranslation(r.Context(), &resp, me)
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleListComments(w http.ResponseWriter, r *http.Request) {
	viewer := s.optionalUser(r)

	p := s.resolveAccessiblePost(w, r, viewer)
	if p == nil {
		return
	}

	limit, ok := parseListLimit(r.URL.Query().Get("limit"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	cur, ok := parseCommentCursor(r.URL.Query().Get("cursor"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid cursor")
		return
	}

	rows, err := s.comments.ListByPost(r.Context(), p.ID, cur, limit+1)
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

	items := make([]commentResponse, 0, len(rows))
	for _, it := range rows {
		resp := commentResponse{
			ID: it.ID,
			Author: commentAuthorResponse{
				ID:          it.AuthorID,
				Username:    it.AuthorUsername,
				DisplayName: it.AuthorDisplayName,
				AvatarURL:   it.AuthorAvatarURL,
			},
			Content:   it.Content,
			CreatedAt: it.CreatedAt.Format(time.RFC3339),
			UpdatedAt: it.UpdatedAt.Format(time.RFC3339),
		}
		// Each comment is translated independently for the viewer.
		s.applyCommentTranslation(r.Context(), &resp, viewer)
		items = append(items, resp)
	}

	writeJSON(w, http.StatusOK, commentListResponse{Items: items, NextCursor: next})
}

// ownedComment loads the comment at {id} and enforces UUID validity, existence
// and authorship. On any failure it writes the response (400/404/500) and
// returns nil. A non-author gets 404 (not 403) so foreign comments aren't
// revealed.
func (s *Server) ownedComment(w http.ResponseWriter, r *http.Request, me *user.User) *comment.Comment {
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, "invalid comment id")
		return nil
	}

	c, err := s.comments.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, comment.ErrNotFound) {
			writeError(w, http.StatusNotFound, "comment not found")
			return nil
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil
	}
	if c.AuthorID != me.ID {
		writeError(w, http.StatusNotFound, "comment not found")
		return nil
	}
	return c
}

func (s *Server) handleUpdateComment(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	c := s.ownedComment(w, r, me)
	if c == nil {
		return
	}

	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "content-type must be application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxCommentBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req createCommentRequest
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	if utf8.RuneCountInString(content) > maxCommentContentRunes {
		writeError(w, http.StatusBadRequest, "content is too long")
		return
	}

	updated, err := s.comments.UpdateContent(r.Context(), c.ID, content)
	if err != nil {
		if errors.Is(err, comment.ErrNotFound) {
			writeError(w, http.StatusNotFound, "comment not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := commentResponse{
		ID: updated.ID,
		Author: commentAuthorResponse{
			ID:          me.ID,
			Username:    me.Username,
			DisplayName: me.DisplayName,
			AvatarURL:   me.AvatarURL,
		},
		Content:   updated.Content,
		CreatedAt: updated.CreatedAt.Format(time.RFC3339),
		UpdatedAt: updated.UpdatedAt.Format(time.RFC3339),
	}
	s.applyCommentTranslation(r.Context(), &resp, me)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	c := s.ownedComment(w, r, me)
	if c == nil {
		return
	}

	if err := s.comments.Delete(r.Context(), c.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// parseCommentCursor decodes a comment cursor. Empty -> nil. Invalid -> false.
func parseCommentCursor(raw string) (*comment.Cursor, bool) {
	if raw == "" {
		return nil, true
	}
	t, id, ok := decodeCursor(raw)
	if !ok {
		return nil, false
	}
	return &comment.Cursor{CreatedAt: t, ID: id}, true
}
