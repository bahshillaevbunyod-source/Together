package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

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

	if err := s.posts.Delete(r.Context(), p.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
