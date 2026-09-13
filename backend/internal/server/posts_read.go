package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"time"

	"together/backend/internal/media"
	"together/backend/internal/post"
	"together/backend/internal/translation"
	"together/backend/internal/user"
)

var uuidPattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`,
)

func (s *Server) handleGetPost(w http.ResponseWriter, r *http.Request) {
	viewer := s.optionalUser(r)

	// Same UUID / existence / visibility / block check as like/unlike.
	p := s.resolveAccessiblePost(w, r, viewer)
	if p == nil {
		return
	}

	author, err := s.users.GetByID(r.Context(), p.AuthorID)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			writeError(w, http.StatusNotFound, "post not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp, err := s.buildPostResponse(r.Context(), p, author, viewer)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// buildPostResponse assembles a safe post response with like/comment counts.
// viewer may be nil (anonymous); likedByMe is then false.
func (s *Server) buildPostResponse(ctx context.Context, p *post.Post, author *user.User, viewer *user.User) (postResponse, error) {
	resp := toPostResponse(p, author)

	likesCount, err := s.likes.CountPostLikes(ctx, p.ID)
	if err != nil {
		return resp, err
	}
	resp.LikesCount = likesCount

	commentsCount, err := s.comments.CountByPost(ctx, p.ID)
	if err != nil {
		return resp, err
	}
	resp.CommentsCount = commentsCount

	if viewer != nil {
		liked, err := s.likes.IsPostLiked(ctx, viewer.ID, p.ID)
		if err != nil {
			return resp, err
		}
		resp.LikedByMe = liked

		saved, err := s.bookmarks.IsSaved(ctx, viewer.ID, p.ID)
		if err != nil {
			return resp, err
		}
		resp.SavedByMe = saved
	}

	mediaItems, err := s.media.ListByPost(ctx, p.ID)
	if err != nil {
		return resp, err
	}
	resp.Media = s.toMediaResponses(mediaItems)

	s.applyPostTranslation(ctx, &resp, viewer)

	return resp, nil
}

// applyPostTranslation fills the translation fields of resp using the viewer's
// preferences. It is a no-op when the viewer is anonymous, opted out, has no
// preferred language, or the post has no text. A translation failure is
// swallowed so the post/feed response still succeeds with the original content.
func (s *Server) applyPostTranslation(ctx context.Context, resp *postResponse, viewer *user.User) {
	if viewer == nil || !viewer.AutoTranslateEnabled || viewer.PreferredLanguage == nil {
		return
	}
	if resp.Content == nil || *resp.Content == "" {
		return
	}
	res, err := s.translator.Translate(ctx, translation.Request{Text: *resp.Content, TargetLang: *viewer.PreferredLanguage})
	if err != nil {
		return
	}
	resp.TranslatedContent = &res.TranslatedText
	resp.SourceLanguage = &res.SourceLang
	resp.TargetLanguage = &res.TargetLang
}

// canViewPost is the single post access check shared by read and like/unlike.
// It enforces visibility, follow relationship and blocks (either direction).
// viewer may be nil (anonymous). A block hides the post entirely, and the
// caller responds 404 so the reason is never revealed.
func (s *Server) canViewPost(ctx context.Context, viewer *user.User, p *post.Post) (bool, error) {
	// A block in either direction hides the post — checked before visibility
	// so even public posts are hidden between blocked users.
	if viewer != nil {
		blocked, err := s.blocks.HasBlockBetween(ctx, viewer.ID, p.AuthorID)
		if err != nil {
			return false, err
		}
		if blocked {
			return false, nil
		}
	}

	switch p.Visibility {
	case post.VisibilityPublic:
		return true, nil
	case post.VisibilityPrivate:
		return viewer != nil && viewer.ID == p.AuthorID, nil
	case post.VisibilityFollowers:
		if viewer == nil {
			return false, nil
		}
		if viewer.ID == p.AuthorID {
			return true, nil
		}
		return s.follows.IsFollowing(ctx, viewer.ID, p.AuthorID)
	default:
		return false, nil
	}
}

func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
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

	// Fetch one extra row to detect whether another page exists.
	rows, err := s.posts.ListFeed(r.Context(), me.ID, cur, limit+1)
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

	// Fetch media for all feed posts in a single batch query (no N+1).
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

	// Bookmark state for all feed posts in a single query (no N+1).
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
		// Each feed item is translated independently for the viewer.
		s.applyPostTranslation(r.Context(), &resp, me)
		items = append(items, resp)
	}

	writeJSON(w, http.StatusOK, feedResponse{Items: items, NextCursor: next})
}

type feedResponse struct {
	Items      []postResponse `json:"items"`
	NextCursor string         `json:"nextCursor"`
}

func feedItemToResponse(it post.FeedItem) postResponse {
	return postResponse{
		ID: it.ID,
		Author: postAuthorResponse{
			ID:          it.AuthorID,
			Username:    it.AuthorUsername,
			DisplayName: it.AuthorDisplayName,
			AvatarURL:   it.AuthorAvatarURL,
		},
		Content:       it.Content,
		Visibility:    it.Visibility,
		CreatedAt:     it.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     it.UpdatedAt.Format(time.RFC3339),
		LikesCount:    it.LikesCount,
		LikedByMe:     it.LikedByMe,
		CommentsCount: it.CommentsCount,
		Media:         []mediaResponse{},
	}
}

// parseFeedCursor decodes a feed cursor. Empty -> nil. Invalid -> ok=false.
func parseFeedCursor(raw string) (*post.Cursor, bool) {
	if raw == "" {
		return nil, true
	}
	t, id, ok := decodeCursor(raw)
	if !ok {
		return nil, false
	}
	return &post.Cursor{CreatedAt: t, ID: id}, true
}
