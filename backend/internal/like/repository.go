// Package like manages post likes.
package like

import "context"

// Repository abstracts like persistence so handlers never touch SQL.
type Repository interface {
	// LikePost likes a post. Idempotent.
	LikePost(ctx context.Context, userID, postID string) error
	// UnlikePost removes a like. Idempotent.
	UnlikePost(ctx context.Context, userID, postID string) error
	// IsPostLiked reports whether userID likes postID.
	IsPostLiked(ctx context.Context, userID, postID string) (bool, error)
	// CountPostLikes returns the number of likes on postID.
	CountPostLikes(ctx context.Context, postID string) (int64, error)
}
