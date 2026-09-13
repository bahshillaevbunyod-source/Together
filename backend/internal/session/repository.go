package session

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when no active session matches.
var ErrNotFound = errors.New("session not found")

// Repository abstracts session persistence so handlers never touch SQL.
type Repository interface {
	// Create stores a new session keyed by the token hash.
	Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) (*Session, error)
	// GetActiveByTokenHash returns a non-expired session by token hash, or
	// ErrNotFound.
	GetActiveByTokenHash(ctx context.Context, tokenHash string) (*Session, error)
	// DeleteByTokenHash removes a session. Deleting a missing session is a
	// no-op (idempotent), not an error.
	DeleteByTokenHash(ctx context.Context, tokenHash string) error
}
