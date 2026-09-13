// Package db manages the PostgreSQL connection pool.
package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// New creates a PostgreSQL connection pool from the given DATABASE_URL.
// The pool connects lazily; use Ping to verify reachability. The caller owns
// the returned pool and must Close it on shutdown.
func New(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	return pool, nil
}
