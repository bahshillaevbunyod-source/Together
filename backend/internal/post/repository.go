package post

import "context"

// Repository abstracts post persistence so handlers never touch SQL.
type Repository interface {
	Create(ctx context.Context, in CreateInput) (*Post, error)
	// CreateRepost creates a provenance-preserving reference to an accessible
	// public original. A duplicate author/original pair returns ErrAlreadyShared.
	CreateRepost(ctx context.Context, authorID, originalPostID string) (*Post, error)
	// GetByID returns the post by id or ErrNotFound.
	GetByID(ctx context.Context, id string) (*Post, error)
	// Update applies a partial update and returns the updated row, or
	// ErrNotFound.
	Update(ctx context.Context, id string, in PostUpdate) (*Post, error)
	// Delete removes the post (comments/likes cascade via FK).
	Delete(ctx context.Context, id string) error
	// ListFeed returns feed items for the given viewer, keyset-paginated:
	// the viewer's own posts plus public/followers posts of users they follow,
	// excluding posts from users with a block in either direction.
	ListFeed(ctx context.Context, viewerID string, cur *Cursor, limit int) ([]FeedItem, error)
	// ListDiscover returns public posts for discovery, keyset-paginated: public
	// posts from other users the viewer does NOT follow, excluding the viewer's
	// own posts and any author in a block relationship (either direction).
	// Newest first (created_at DESC, id DESC).
	ListDiscover(ctx context.Context, viewerID string, cur *Cursor, limit int) ([]FeedItem, error)
	// ListTopicPosts returns posts associated with topicID which are visible to
	// viewerID. A nil viewer sees public posts only. Results are keyset-paginated
	// newest first and exclude blocked relationships in either direction.
	ListTopicPosts(ctx context.Context, viewerID *string, topicID string, cur *Cursor, limit int) ([]FeedItem, error)
}
