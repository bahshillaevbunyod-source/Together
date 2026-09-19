package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"together/backend/internal/story"
)

// --- response shapes -----------------------------------------------------

type storyAuthorResponse struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"displayName"`
	AvatarURL   *string `json:"avatarUrl"`
}

// storyMediaResponse never exposes storage_key or a raw bucket URL. `url` is a
// short-lived presigned GET minted only after the story's access was verified.
type storyMediaResponse struct {
	Type       string `json:"type"`
	URL        string `json:"url"`
	MimeType   string `json:"mimeType"`
	Width      *int   `json:"width"`
	Height     *int   `json:"height"`
	DurationMs *int64 `json:"durationMs"`
}

type storyResponse struct {
	ID        string              `json:"id"`
	Author    storyAuthorResponse `json:"author"`
	Media     storyMediaResponse  `json:"media"`
	CreatedAt string              `json:"createdAt"`
	Viewed    bool                `json:"viewed"`
}

type storyListResponse struct {
	Items      []storyResponse `json:"items"`
	NextCursor string          `json:"nextCursor"`
}

// storyItemToResponse builds the API shape from an authorized story item. The
// media URL is derived here (presigned for private media) — callers must only
// pass items the viewer is already authorized to see.
//
// Signing-failure convention (matches post media / mediaURLForKey): if the
// presign fails, Media.URL is "" — an empty, non-usable value. It is never a
// public or raw bucket URL, so a signing failure degrades to "media
// unavailable" rather than turning into a misleading usable/leaking URL. The
// story's metadata (type/mimeType/…) is still returned honestly.
func (s *Server) storyItemToResponse(r *http.Request, it story.Item) storyResponse {
	return storyResponse{
		ID: it.ID,
		Author: storyAuthorResponse{
			ID:          it.AuthorID,
			Username:    it.AuthorUsername,
			DisplayName: it.AuthorDisplayName,
			AvatarURL:   it.AuthorAvatarURL,
		},
		Media: storyMediaResponse{
			Type:       it.Type,
			URL:        s.mediaURLForKey(r.Context(), it.StorageKey),
			MimeType:   it.MimeType,
			Width:      it.Width,
			Height:     it.Height,
			DurationMs: it.DurationMs,
		},
		CreatedAt: it.CreatedAt.Format(time.RFC3339),
		Viewed:    it.Viewed,
	}
}

// --- create --------------------------------------------------------------

type createStoryRequest struct {
	// StorageKey is a previously confirmed upload that MUST belong to the
	// current user and live in their private namespace. It is validated against
	// storage (ownership, namespace, existence, media policy) — never trusted.
	StorageKey string `json:"storageKey"`
}

func (s *Server) handleCreateStory(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "content-type must be application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req createStoryRequest
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate the referenced upload at create time — the same established
	// pattern post creation uses (resolveMediaForPost also calls
	// validateUploadedObject directly). The /api/v1/media/confirm endpoint is a
	// STATELESS client-side pre-check over this exact primitive; it persists no
	// "confirmed" state, so re-validating here is the real invariant, not a
	// bypass. Stories are ALWAYS private media: allowing only the "private"
	// namespace enforces (a) ownership — the key must be under users/{me}/,
	// (b) private storage class — never a public "uploads"/"avatars" key, and
	// (c) existence + media policy via storage HeadObject. A foreign or
	// arbitrary key fails here.
	confirmed, err := s.validateUploadedObject(r.Context(), me.ID, req.StorageKey, "private")
	if err != nil {
		switch {
		case errors.Is(err, errInvalidStorageKey), errors.Is(err, errUnsupportedMedia):
			writeError(w, http.StatusBadRequest, "invalid media")
		case errors.Is(err, errObjectMissing):
			writeError(w, http.StatusNotFound, "object not found")
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	created, err := s.stories.Create(r.Context(), story.CreateInput{
		AuthorID:   me.ID,
		Type:       confirmed.Type,
		StorageKey: confirmed.StorageKey,
		MimeType:   confirmed.MimeType,
		// Dimensions/duration are not derived from a HEAD and are never taken
		// from the client; they remain null for V1.
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// The author is the current user; a freshly created story is unviewed.
	resp := storyResponse{
		ID: created.ID,
		Author: storyAuthorResponse{
			ID:          me.ID,
			Username:    me.Username,
			DisplayName: me.DisplayName,
			AvatarURL:   me.AvatarURL,
		},
		Media: storyMediaResponse{
			Type:       created.Type,
			URL:        s.mediaURLForKey(r.Context(), created.StorageKey),
			MimeType:   created.MimeType,
			Width:      created.Width,
			Height:     created.Height,
			DurationMs: created.DurationMs,
		},
		CreatedAt: created.CreatedAt.Format(time.RFC3339),
		Viewed:    false,
	}
	writeJSON(w, http.StatusCreated, resp)
}

// --- feed ----------------------------------------------------------------

func (s *Server) handleListStoriesFeed(w http.ResponseWriter, r *http.Request) {
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
	cur, ok := parseStoryCursor(r.URL.Query().Get("cursor"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid cursor")
		return
	}

	// Access (self + accepted follows, no block, no pending requests) and the
	// 24-hour active window are enforced entirely in the repository query.
	rows, err := s.stories.ListFeed(r.Context(), me.ID, cur, limit+1)
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

	items := make([]storyResponse, 0, len(rows))
	for _, it := range rows {
		items = append(items, s.storyItemToResponse(r, it))
	}
	writeJSON(w, http.StatusOK, storyListResponse{Items: items, NextCursor: next})
}

// --- user's stories ------------------------------------------------------

func (s *Server) handleUserStories(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	target := s.resolveTarget(w, r)
	if target == nil {
		return
	}
	// Blocked pairs are hidden as "not found", matching the followers/following
	// list and public-profile conventions (never reveals the relationship).
	if s.rejectBlockedTarget(w, r, me, target) {
		return
	}

	// ListAuthorActive enforces the access rule in SQL (self-or-accepted-follows,
	// no block, active window). A viewer without access simply gets an empty
	// list, identical to "no active stories" — so inaccessible existence is
	// never leaked.
	rows, err := s.stories.ListAuthorActive(r.Context(), me.ID, target.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	items := make([]storyResponse, 0, len(rows))
	for _, it := range rows {
		items = append(items, s.storyItemToResponse(r, it))
	}
	writeJSON(w, http.StatusOK, storyListResponse{Items: items, NextCursor: ""})
}

// --- single story --------------------------------------------------------

func (s *Server) handleGetStory(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		// A syntactically invalid id cannot name a real story; treat as absent
		// so nothing is leaked.
		writeError(w, http.StatusNotFound, "story not found")
		return
	}

	it, err := s.stories.GetActiveVisible(r.Context(), me.ID, id)
	if err != nil {
		// ErrNotFound covers nonexistent, expired, blocked, and no-access alike,
		// so private existence is never revealed.
		if errors.Is(err, story.ErrNotFound) {
			writeError(w, http.StatusNotFound, "story not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, s.storyItemToResponse(r, *it))
}

// --- record view ---------------------------------------------------------

func (s *Server) handleViewStory(w http.ResponseWriter, r *http.Request) {
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

	// A view is recorded only for a story the viewer is authorized to see AND
	// that is still active. If the story is inaccessible/expired/nonexistent,
	// GetActiveVisible returns ErrNotFound and no story_views row is created.
	if _, err := s.stories.GetActiveVisible(r.Context(), me.ID, id); err != nil {
		if errors.Is(err, story.ErrNotFound) {
			writeError(w, http.StatusNotFound, "story not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Idempotent: a repeat view (including the author viewing their own story)
	// creates no duplicate row.
	if _, err := s.stories.RecordView(r.Context(), id, me.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"viewed": true})
}

// --- delete --------------------------------------------------------------

func (s *Server) handleDeleteStory(w http.ResponseWriter, r *http.Request) {
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

	// Owner-only: DeleteOwn deletes only when the story belongs to the caller.
	// A non-owner (or nonexistent id) deletes nothing and receives 404, so
	// another user's story is never revealed. FK ON DELETE CASCADE removes the
	// story's story_views rows. R2 object deletion is intentionally out of scope
	// (see stories.go note): the private object is orphaned and should be reaped
	// by a future lifecycle/cleanup step, never by ad-hoc deletion here.
	deleted, err := s.stories.DeleteOwn(r.Context(), me.ID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "story not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// parseStoryCursor decodes a stories feed cursor. Empty -> nil (first page).
// Invalid -> ok=false.
func parseStoryCursor(raw string) (*story.Cursor, bool) {
	if raw == "" {
		return nil, true
	}
	t, id, ok := decodeCursor(raw)
	if !ok || !uuidPattern.MatchString(id) {
		return nil, false
	}
	return &story.Cursor{CreatedAt: t, ID: id}, true
}
