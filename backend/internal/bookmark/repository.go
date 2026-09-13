// Package bookmark manages users' saved posts.
package bookmark

import (
	"context"
	"time"

	"together/backend/internal/post"
)

// SavedItem is a bookmarked post with its display data. It embeds post.FeedItem
// (post fields, author, counts) and adds the bookmark time for cursoring.
type SavedItem struct {
	post.FeedItem
	BookmarkedAt time.Time
}

// Repository abstracts bookmark persistence so handlers never touch SQL.
type Repository interface {
	// Save bookmarks a post for a user. Idempotent.
	Save(ctx context.Context, userID, postID string) error
	// Remove deletes a bookmark. Idempotent.
	Remove(ctx context.Context, userID, postID string) error
	// IsSaved reports whether the user has bookmarked the post.
	IsSaved(ctx context.Context, userID, postID string) (bool, error)
	// ListSavedPostIDs returns which of postIDs the user has bookmarked, in one
	// query (feed, no N+1). Empty postIDs returns an empty map without SQL.
	ListSavedPostIDs(ctx context.Context, userID string, postIDs []string) (map[string]bool, error)
	// ListSaved returns the user's bookmarked, accessible posts (visibility +
	// block enforced), newest-bookmark-first, keyset-paginated. Counts are
	// computed in the same query (no N+1).
	ListSaved(ctx context.Context, userID string, cur *post.Cursor, limit int) ([]SavedItem, error)
}
