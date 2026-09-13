package session

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository is the production Repository backed by pgxpool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// Compile-time assurance that the production repository satisfies Repository.
var _ Repository = (*PostgresRepository)(nil)

// NewPostgresRepository builds a Repository over the given connection pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const insertSessionQuery = `
INSERT INTO sessions (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING id, user_id, token_hash, expires_at, created_at
`

// Create stores a new session and returns the stored row.
func (r *PostgresRepository) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) (*Session, error) {
	row := r.pool.QueryRow(ctx, insertSessionQuery, userID, tokenHash, expiresAt)

	var s Session
	if err := row.Scan(&s.ID, &s.UserID, &s.TokenHash, &s.ExpiresAt, &s.CreatedAt); err != nil {
		return nil, err
	}
	return &s, nil
}

const getActiveSessionQuery = `
SELECT id, user_id, token_hash, expires_at, created_at
FROM sessions
WHERE token_hash = $1 AND expires_at > now()
`

// GetActiveByTokenHash returns a non-expired session by token hash, or
// ErrNotFound.
func (r *PostgresRepository) GetActiveByTokenHash(ctx context.Context, tokenHash string) (*Session, error) {
	row := r.pool.QueryRow(ctx, getActiveSessionQuery, tokenHash)

	var s Session
	if err := row.Scan(&s.ID, &s.UserID, &s.TokenHash, &s.ExpiresAt, &s.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}

const deleteSessionQuery = `DELETE FROM sessions WHERE token_hash = $1`

// DeleteByTokenHash removes a session by its token hash. Deleting zero rows is
// treated as success (idempotent).
func (r *PostgresRepository) DeleteByTokenHash(ctx context.Context, tokenHash string) error {
	_, err := r.pool.Exec(ctx, deleteSessionQuery, tokenHash)
	return err
}
