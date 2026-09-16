package topic

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrNotFound is returned when a canonical topic slug has no row.
var ErrNotFound = errors.New("topic not found")

// Topic is a canonical hashtag topic.
type Topic struct {
	ID        string
	Slug      string
	CreatedAt time.Time
}

// TrendingItem is a topic with the count of posts visible to the viewer.
type TrendingItem struct {
	Slug       string
	PostsCount int64
}

// DBTX is the minimal executor needed to persist topics inside a transaction.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repository persists canonical topics and their post relationships.
type Repository interface {
	CreatePostTopicsTx(ctx context.Context, q DBTX, postID string, slugs []string) error
	GetBySlug(ctx context.Context, slug string) (*Topic, error)
	ListTrending(ctx context.Context, viewerID *string, limit int) ([]TrendingItem, error)
}
