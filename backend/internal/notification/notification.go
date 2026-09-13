// Package notification stores user notifications (follow, post_like,
// post_comment) and their persistence.
package notification

import "time"

// Notification types allowed by the notifications_type_valid constraint.
const (
	TypeFollow      = "follow"
	TypePostLike    = "post_like"
	TypePostComment = "post_comment"
)

// Notification mirrors a row in the `notifications` table.
type Notification struct {
	ID        string
	UserID    string
	ActorID   *string
	Type      string
	PostID    *string
	CommentID *string
	ReadAt    *time.Time
	CreatedAt time.Time
}

// CreateInput carries the fields to create a notification.
type CreateInput struct {
	UserID    string
	ActorID   *string
	Type      string
	PostID    *string
	CommentID *string
}

// Cursor is a stable keyset position: created_at plus id.
type Cursor struct {
	CreatedAt time.Time
	ID        string
}

// ListItem is a notification joined with its actor's public-safe fields.
// Actor fields are nil when the actor was removed (actor_id ON DELETE SET NULL).
type ListItem struct {
	ID               string
	Type             string
	PostID           *string
	CommentID        *string
	ReadAt           *time.Time
	CreatedAt        time.Time
	ActorID          *string
	ActorUsername    *string
	ActorDisplayName *string
	ActorAvatarURL   *string
}
