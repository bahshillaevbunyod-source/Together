package topic

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX is the minimal executor needed to persist topics inside a transaction.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repository persists canonical topics and their post relationships.
type Repository interface {
	CreatePostTopicsTx(ctx context.Context, q DBTX, postID string, slugs []string) error
}
