// Package postservice provides transactional use-cases spanning posts and their
// media, so a post and its media rows are created atomically.
package postservice

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"together/backend/internal/media"
	"together/backend/internal/post"
)

// Tx is a transaction usable as a query executor for both repositories.
type Tx interface {
	post.DBTX
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// Beginner opens transactions.
type Beginner interface {
	Begin(ctx context.Context) (Tx, error)
}

// PostCreator creates a post within a transaction.
type PostCreator interface {
	CreateTx(ctx context.Context, q post.DBTX, in post.CreateInput) (*post.Post, error)
}

// MediaCreator creates media rows within a transaction.
type MediaCreator interface {
	CreateManyTx(ctx context.Context, q media.DBTX, items []media.CreateInput) error
}

// Service creates posts with media atomically.
type Service struct {
	db    Beginner
	posts PostCreator
	media MediaCreator
}

// New builds a Service. The pool is wrapped so its transactions satisfy Tx.
func New(pool *pgxpool.Pool, posts PostCreator, mediaRepo MediaCreator) *Service {
	return &Service{db: poolBeginner{pool: pool}, posts: posts, media: mediaRepo}
}

// CreatePostWithMedia creates a post and its media in a single transaction.
// mediaItems' PostID is set from the created post (any client-supplied value is
// ignored). An empty mediaItems list is valid. On any error the transaction is
// rolled back and the error is returned. Media are assumed already validated.
func (s *Service) CreatePostWithMedia(ctx context.Context, in post.CreateInput, mediaItems []media.CreateInput) (*post.Post, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) // no-op after a successful Commit

	p, err := s.posts.CreateTx(ctx, tx, in)
	if err != nil {
		return nil, err
	}

	items := make([]media.CreateInput, len(mediaItems))
	for i, m := range mediaItems {
		m.PostID = p.ID
		items[i] = m
	}
	if err := s.media.CreateManyTx(ctx, tx, items); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

// poolBeginner adapts *pgxpool.Pool to Beginner (pgx.Tx satisfies Tx).
type poolBeginner struct {
	pool *pgxpool.Pool
}

func (b poolBeginner) Begin(ctx context.Context) (Tx, error) {
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return tx, nil
}
