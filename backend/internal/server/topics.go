package server

import (
	"errors"
	"net/http"
	"strings"

	"together/backend/internal/media"
	"together/backend/internal/post"
	"together/backend/internal/topic"
	"together/backend/internal/user"
)

type topicResponse struct {
	Slug string `json:"slug"`
}

type trendingTopicResponse struct {
	Slug       string `json:"slug"`
	PostsCount int64  `json:"postsCount"`
}

type trendingTopicsResponse struct {
	Items []trendingTopicResponse `json:"items"`
}

const topicSearchLimit = 10

// handleListTopics returns a simple popularity ranking. The repository counts
// only posts visible to the optional viewer, including block filtering.
func (s *Server) handleListTopics(w http.ResponseWriter, r *http.Request) {
	if s.topics == nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	limit, ok := parseListLimit(r.URL.Query().Get("limit"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}

	var viewerID *string
	if viewer := s.optionalUser(r); viewer != nil {
		viewerID = &viewer.ID
	}
	items, err := s.topics.ListTrending(r.Context(), viewerID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	response := make([]trendingTopicResponse, 0, len(items))
	for _, item := range items {
		response = append(response, trendingTopicResponse{Slug: item.Slug, PostsCount: item.PostsCount})
	}
	writeJSON(w, http.StatusOK, trendingTopicsResponse{Items: response})
}

// handleSearchTopics searches canonical slugs within the set of topics whose
// posts are visible to the optional viewer. It intentionally has a small fixed
// limit because this endpoint backs type-ahead search.
func (s *Server) handleSearchTopics(w http.ResponseWriter, r *http.Request) {
	if s.topics == nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rawQuery := strings.TrimPrefix(r.URL.Query().Get("q"), "#")
	query, ok := topic.CanonicalSlug(rawQuery)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid query")
		return
	}

	var viewerID *string
	if viewer := s.optionalUser(r); viewer != nil {
		viewerID = &viewer.ID
	}
	items, err := s.topics.Search(r.Context(), viewerID, query, topicSearchLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	response := make([]trendingTopicResponse, 0, len(items))
	for _, item := range items {
		response = append(response, trendingTopicResponse{Slug: item.Slug, PostsCount: item.PostsCount})
	}
	writeJSON(w, http.StatusOK, trendingTopicsResponse{Items: response})
}

// handleGetTopic looks up a canonical topic without disclosing a topic that
// currently has no posts visible to the requester.
func (s *Server) handleGetTopic(w http.ResponseWriter, r *http.Request) {
	item, viewer, ok := s.lookupVisibleTopic(w, r)
	if !ok {
		return
	}

	rows, err := s.posts.ListTopicPosts(r.Context(), viewerID(viewer), item.ID, nil, 1)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if len(rows) == 0 {
		writeError(w, http.StatusNotFound, "topic not found")
		return
	}
	writeJSON(w, http.StatusOK, topicResponse{Slug: item.Slug})
}

// handleTopicPosts returns visible posts for a topic in the established feed
// response shape, with the same deterministic keyset cursor as feed/discover.
func (s *Server) handleTopicPosts(w http.ResponseWriter, r *http.Request) {
	item, viewer, ok := s.lookupVisibleTopic(w, r)
	if !ok {
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

	rows, err := s.posts.ListTopicPosts(r.Context(), viewerID(viewer), item.ID, cur, limit+1)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if len(rows) == 0 {
		writeError(w, http.StatusNotFound, "topic not found")
		return
	}

	next := ""
	if len(rows) > limit {
		last := rows[limit-1]
		next = encodeCursor(last.CreatedAt, last.ID)
		rows = rows[:limit]
	}
	items, err := s.topicPostResponses(r, rows, viewer)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, feedResponse{Items: items, NextCursor: next})
}

func (s *Server) lookupVisibleTopic(w http.ResponseWriter, r *http.Request) (*topic.Topic, *user.User, bool) {
	if s.topics == nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil, nil, false
	}
	slug, ok := topic.CanonicalSlug(r.PathValue("slug"))
	if !ok {
		writeError(w, http.StatusNotFound, "topic not found")
		return nil, nil, false
	}
	item, err := s.topics.GetBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, topic.ErrNotFound) {
			writeError(w, http.StatusNotFound, "topic not found")
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return nil, nil, false
	}
	return item, s.optionalUser(r), true
}

func viewerID(viewer *user.User) *string {
	if viewer == nil {
		return nil
	}
	return &viewer.ID
}

func (s *Server) topicPostResponses(r *http.Request, rows []post.FeedItem, viewer *user.User) ([]postResponse, error) {
	postIDs := make([]string, len(rows))
	for i, row := range rows {
		postIDs[i] = row.ID
	}
	mediaItems, err := s.media.ListByPostIDs(r.Context(), postIDs)
	if err != nil {
		return nil, err
	}
	mediaByPost := make(map[string][]media.Media)
	for _, item := range mediaItems {
		mediaByPost[item.PostID] = append(mediaByPost[item.PostID], item)
	}

	savedByPost := map[string]bool{}
	if viewer != nil {
		savedByPost, err = s.bookmarks.ListSavedPostIDs(r.Context(), viewer.ID, postIDs)
		if err != nil {
			return nil, err
		}
	}

	items := make([]postResponse, 0, len(rows))
	for _, row := range rows {
		response := feedItemToResponse(row)
		response.Media = s.toMediaResponses(r.Context(), mediaByPost[row.ID])
		response.SavedByMe = savedByPost[row.ID]
		s.applyPostTranslation(r.Context(), &response, viewer)
		items = append(items, response)
	}
	return items, nil
}
