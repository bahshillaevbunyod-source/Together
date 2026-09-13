// Package follow manages the follow graph between users.
package follow

import (
	"context"
	"time"
)

// Cursor is a stable keyset position: the follow edge's created_at plus the
// listed user's id. Ordering is (created_at DESC, user_id DESC).
type Cursor struct {
	CreatedAt time.Time
	UserID    string
}

// ListItem is one public-safe user in a followers/following list. CreatedAt is
// the follow edge time, used only to build the next cursor.
type ListItem struct {
	ID             string
	Username       string
	DisplayName    string
	AvatarURL      *string
	CountryCode    *string
	City           *string
	NativeLanguage string
	CreatedAt      time.Time
}

// Repository abstracts follow persistence so handlers never touch SQL.
type Repository interface {
	// Follow makes followerID follow followingID. Idempotent.
	Follow(ctx context.Context, followerID, followingID string) error
	// Unfollow removes the follow edge. Idempotent.
	Unfollow(ctx context.Context, followerID, followingID string) error
	// IsFollowing reports whether followerID currently follows followingID.
	IsFollowing(ctx context.Context, followerID, followingID string) (bool, error)
	// CountFollowers returns how many users follow userID.
	CountFollowers(ctx context.Context, userID string) (int64, error)
	// CountFollowing returns how many users userID follows.
	CountFollowing(ctx context.Context, userID string) (int64, error)
	// ListFollowers returns up to `limit` users who follow userID, ordered by
	// the keyset cursor. Pass nil cursor for the first page.
	ListFollowers(ctx context.Context, userID string, cur *Cursor, limit int) ([]ListItem, error)
	// ListFollowing returns up to `limit` users userID follows.
	ListFollowing(ctx context.Context, userID string, cur *Cursor, limit int) ([]ListItem, error)
}
