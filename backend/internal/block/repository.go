// Package block manages user blocks.
package block

import "context"

// Repository abstracts block persistence so handlers never touch SQL.
type Repository interface {
	// Block makes blockerID block blockedID and removes any follow edges
	// between them, atomically. Idempotent.
	Block(ctx context.Context, blockerID, blockedID string) error
	// Unblock removes the block edge. Idempotent.
	Unblock(ctx context.Context, blockerID, blockedID string) error
	// IsBlocked reports whether blockerID blocks blockedID (one direction).
	IsBlocked(ctx context.Context, blockerID, blockedID string) (bool, error)
	// HasBlockBetween reports whether either user blocks the other.
	HasBlockBetween(ctx context.Context, userA, userB string) (bool, error)
	// List returns the users blocked by blockerID. It never returns incoming
	// blocks, so a caller can only inspect their own safety choices.
	List(ctx context.Context, blockerID string, limit int) ([]ListItem, error)
}

// ListItem is the public-safe identity shown in a user's blocked-people list.
type ListItem struct {
	ID          string
	Username    string
	DisplayName string
	AvatarURL   *string
}
