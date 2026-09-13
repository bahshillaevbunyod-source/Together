package server

import (
	"net/http"

	"together/backend/internal/media"
)

func (s *Server) handleListBookmarks(w http.ResponseWriter, r *http.Request) {
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
	cur, ok := parseFeedCursor(r.URL.Query().Get("cursor"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid cursor")
		return
	}

	// Access rules (visibility + block) are enforced inside ListSaved's query.
	rows, err := s.bookmarks.ListSaved(r.Context(), me.ID, cur, limit+1)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	next := ""
	if len(rows) > limit {
		last := rows[limit-1]
		next = encodeCursor(last.BookmarkedAt, last.ID)
		rows = rows[:limit]
	}

	// Batch media for all posts in one query (no N+1). Likes/comment counts are
	// already computed in ListSaved.
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

	items := make([]postResponse, 0, len(rows))
	for _, it := range rows {
		resp := feedItemToResponse(it.FeedItem)
		resp.Media = s.toMediaResponses(mediaByPost[it.ID])
		resp.SavedByMe = true // every item here is bookmarked by the viewer
		items = append(items, resp)
	}

	writeJSON(w, http.StatusOK, feedResponse{Items: items, NextCursor: next})
}
