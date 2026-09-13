package notification

import "context"

// Repository abstracts notification persistence so handlers never touch SQL.
type Repository interface {
	Create(ctx context.Context, in CreateInput) (*Notification, error)
	// CountUnread returns the number of unread notifications for a user.
	CountUnread(ctx context.Context, userID string) (int64, error)
	// List returns a user's notifications newest-first, keyset-paginated.
	List(ctx context.Context, userID string, cur *Cursor, limit int) ([]ListItem, error)
	// MarkRead marks a user's own notification read, preserving the first
	// read_at. Returns whether a matching notification existed.
	MarkRead(ctx context.Context, userID, notificationID string) (bool, error)
	// MarkAllRead marks all of a user's unread notifications read.
	MarkAllRead(ctx context.Context, userID string) error
}
