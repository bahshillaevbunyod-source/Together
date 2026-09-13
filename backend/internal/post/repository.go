package post

import "context"

// Repository abstracts post persistence so handlers never touch SQL.
type Repository interface {
	Create(ctx context.Context, in CreateInput) (*Post, error)
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
}
