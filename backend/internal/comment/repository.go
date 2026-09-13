package comment

import "context"

// Repository abstracts comment persistence so handlers never touch SQL.
type Repository interface {
	Create(ctx context.Context, in CreateInput) (*Comment, error)
	// ListByPost returns up to `limit` comments for a post, keyset-paginated.
	ListByPost(ctx context.Context, postID string, cur *Cursor, limit int) ([]ListItem, error)
	// CountByPost returns the number of comments on a post.
	CountByPost(ctx context.Context, postID string) (int64, error)
	// GetByID returns the comment by id or ErrNotFound.
	GetByID(ctx context.Context, id string) (*Comment, error)
	// UpdateContent updates the comment's content and returns the updated row.
	UpdateContent(ctx context.Context, id, content string) (*Comment, error)
	// Delete removes the comment.
	Delete(ctx context.Context, id string) error
}
