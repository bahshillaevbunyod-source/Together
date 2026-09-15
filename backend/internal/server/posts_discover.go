package server

import (
	"net/http"
	"strconv"

	"together/backend/internal/media"
)

const (
	discoverPostsDefaultLimit = 10
	discoverPostsMaxLimit     = 30
)

// handleDiscoverPosts serves GET /api/v1/posts/discover?limit=&cursor=. Auth
// required. Returns public posts from other users the viewer does not follow
// (excluding own posts and blocked relationships), newest first, in the SAME
// shape as the feed so the existing PostCard renders them unchanged.
func (s *Server) handleDiscoverPosts(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	limit := discoverPostsDefaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = n
	}
	if limit > discoverPostsMaxLimit {
		limit = discoverPostsMaxLimit
	}

	cur, ok := parseFeedCursor(r.URL.Query().Get("cursor"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid cursor")
		return
	}

	// Fetch one extra row to detect whether another page exists.
	rows, err := s.posts.ListDiscover(r.Context(), me.ID, cur, limit+1)
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

	// Batch media for all posts (no N+1).
	postIDs := make([]string, len(rows))
	for i, it := range rows {
		postIDs[i] = it.ID
	}
	mediaItems, err := s.media.ListByPostIDs(r.Context(), postIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	mediaByPost := make(map[string][]media.Media)
	for _, m := range mediaItems {
		mediaByPost[m.PostID] = append(mediaByPost[m.PostID], m)
	}

	// Bookmark state for all posts in one query (no N+1).
	savedByPost, err := s.bookmarks.ListSavedPostIDs(r.Context(), me.ID, postIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	items := make([]postResponse, 0, len(rows))
	for _, it := range rows {
		resp := feedItemToResponse(it)
		resp.Media = s.toMediaResponses(mediaByPost[it.ID])
		resp.SavedByMe = savedByPost[it.ID]
		s.applyPostTranslation(r.Context(), &resp, me)
		items = append(items, resp)
	}

	writeJSON(w, http.StatusOK, feedResponse{Items: items, NextCursor: next})
}
