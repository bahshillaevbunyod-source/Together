package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"together/backend/internal/media"
	"together/backend/internal/post"
	"together/backend/internal/user"
)

var editablePostFields = map[string]bool{
	"content":    true,
	"visibility": true,
}

// ownedPost loads the post at {id} and enforces UUID validity, existence and
// authorship. On failure it writes the response (400/404/500) and returns nil.
// A non-author gets 404 (not 403).
func (s *Server) ownedPost(w http.ResponseWriter, r *http.Request, me *user.User) *post.Post {
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, "invalid post id")
		return nil
	}

	p, err := s.posts.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, post.ErrNotFound) {
			writeError(w, http.StatusNotFound, "post not found")
			return nil
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil
	}
	if p.AuthorID != me.ID {
		writeError(w, http.StatusNotFound, "post not found")
		return nil
	}
	return p
}

func (s *Server) handleUpdatePost(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	p := s.ownedPost(w, r, me)
	if p == nil {
		return
	}

	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "content-type must be application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPostBodyBytes)
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(raw) == 0 {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}
	for key := range raw {
		if !editablePostFields[key] {
			writeError(w, http.StatusBadRequest, "field not editable: "+key)
			return
		}
	}

	update, ok := buildPostUpdate(raw)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid post fields")
		return
	}
	if !update.HasChanges() {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	// A visibility change that crosses storage classes (public <-> restricted)
	// would strand this post's media in the wrong bucket — e.g. a public object
	// staying permanently public after the post becomes followers-only. Media
	// cannot be moved across buckets here (separate credentials), so reject the
	// change when the post has media. Text-only posts change freely.
	if update.Visibility != nil &&
		storageClassForVisibility(*update.Visibility) != storageClassForVisibility(p.Visibility) {
		existing, err := s.media.ListByPost(r.Context(), p.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if len(existing) > 0 {
			writeError(w, http.StatusConflict, "cannot change visibility of a post with media")
			return
		}
	}

	updated, err := s.posts.Update(r.Context(), p.ID, update)
	if err != nil {
		if errors.Is(err, post.ErrNotFound) {
			writeError(w, http.StatusNotFound, "post not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp, err := s.buildPostResponse(r.Context(), updated, me, me)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDeletePost(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	p := s.ownedPost(w, r, me)
	if p == nil {
		return
	}

	// Capture the media storage keys BEFORE deletion: the posts FK cascade
	// removes the media rows, so they are unavailable afterward. A read error
	// here is non-fatal — the post is still deleted; only cleanup is skipped.
	mediaItems, mediaErr := s.media.ListByPost(r.Context(), p.ID)

	if err := s.posts.Delete(r.Context(), p.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Best-effort storage cleanup AFTER the DB delete (the source of truth).
	// Only the owner's own confirmed post-upload objects are removed; a storage
	// failure leaves an orphaned object but never fails the request.
	if mediaErr == nil {
		s.cleanupPostMedia(r.Context(), me.ID, mediaItems)
	}

	w.WriteHeader(http.StatusNoContent)
}

// cleanupPostMedia best-effort deletes a deleted post's media objects from
// storage. It only touches keys inside the owner's own confirmed upload
// namespace (users/{ownerID}/uploads/<file>) — the same namespace enforced at
// media confirmation — so it can never remove another user's object, an avatar,
// or an arbitrary key. Deletion errors are ignored (see the endpoint's
// best-effort contract); keys come from the database, never from the client.
func (s *Server) cleanupPostMedia(ctx context.Context, ownerID string, items []media.Media) {
	uploadsPrefix := "users/" + ownerID + "/uploads/" // public bucket
	privatePrefix := "users/" + ownerID + "/private/" // private bucket
	for _, m := range items {
		key := m.StorageKey
		var prefix string
		switch {
		case strings.HasPrefix(key, uploadsPrefix):
			prefix = uploadsPrefix
		case strings.HasPrefix(key, privatePrefix):
			prefix = privatePrefix
		default:
			continue // outside the owner's post-media namespaces (e.g. avatar) — never delete
		}
		if strings.Contains(key, "..") || strings.Contains(key, `\`) {
			continue // defense-in-depth against traversal-shaped keys
		}
		rest := key[len(prefix):]
		if rest == "" || strings.Contains(rest, "/") {
			continue // must be a single file segment
		}
		// Delete from the bucket the key belongs to (public vs private).
		_ = s.storage.DeleteObject(ctx, storageClassForKey(key), key) // best-effort; orphan on failure
	}
}

// buildPostUpdate validates raw fields into a PostUpdate. ok is false on any
// invalid value.
func buildPostUpdate(raw map[string]json.RawMessage) (post.PostUpdate, bool) {
	var up post.PostUpdate

	if v, present := raw["content"]; present {
		s, isNull, err := decodeString(v)
		if err != nil || isNull {
			return up, false
		}
		s = strings.TrimSpace(s)
		if s == "" || utf8.RuneCountInString(s) > maxPostContentRunes {
			return up, false
		}
		up.Content = &s
	}

	if v, present := raw["visibility"]; present {
		s, isNull, err := decodeString(v)
		if err != nil || isNull || !validVisibility[s] {
			return up, false
		}
		up.Visibility = &s
	}

	return up, true
}
