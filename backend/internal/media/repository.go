package media

import "context"

// Repository abstracts media persistence so handlers never touch SQL.
// (No DeleteByPost: media rows are removed by the posts FK ON DELETE CASCADE.)
type Repository interface {
	Create(ctx context.Context, in CreateInput) (*Media, error)
	// CreateMany inserts several already-validated media rows, preserving
	// sort_order. An empty list is a no-op (no SQL).
	CreateMany(ctx context.Context, items []CreateInput) error
	// ListByPost returns a post's media ordered by sort_order.
	ListByPost(ctx context.Context, postID string) ([]Media, error)
	// ListByPostIDs returns media for many posts in one query (feed, no N+1),
	// ordered by (post_id, sort_order).
	ListByPostIDs(ctx context.Context, postIDs []string) ([]Media, error)
}
