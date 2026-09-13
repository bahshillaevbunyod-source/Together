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
}
