// Package likeservice provides the transactional post-like use-case: creating a
// like and, when the like is new and not a self-like, its "post_like"
// notification atomically.
package likeservice

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"together/backend/internal/like"
	"together/backend/internal/notification"
)

// Tx is a transaction usable as a query executor by both repositories.
// notification.DBTX is the wider executor (Exec/QueryRow/Query) and also covers
// like.DBTX's single Exec method.
type Tx interface {
	notification.DBTX
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// Beginner opens transactions.
type Beginner interface {
	Begin(ctx context.Context) (Tx, error)
}

// LikeCreator inserts a like within a transaction, reporting whether a new like
// was created.
type LikeCreator interface {
	LikeTx(ctx context.Context, q like.DBTX, userID, postID string) (bool, error)
}

// NotificationCreator inserts a notification within a transaction.
type NotificationCreator interface {
	CreateTx(ctx context.Context, q notification.DBTX, in notification.CreateInput) error
}

// Service likes a post and records the like notification atomically.
type Service struct {
	db     Beginner
	likes  LikeCreator
	notifs NotificationCreator
}

// New builds a Service. The pool is wrapped so its transactions satisfy Tx.
func New(pool *pgxpool.Pool, likes LikeCreator, notifs NotificationCreator) *Service {
	return &Service{db: poolBeginner{pool: pool}, likes: likes, notifs: notifs}
}

// Like records that actorID likes postID and, only when the like is newly
// created and the actor is not the post's author, records a "post_like"
// notification for the author — both in one transaction. A duplicate like
// creates no duplicate notification; a self-like creates none. The caller is
// responsible for enforcing post visibility/block access before calling this.
// On any error the transaction is rolled back.
func (s *Service) Like(ctx context.Context, actorID, postID, authorID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // no-op after a successful Commit

	created, err := s.likes.LikeTx(ctx, tx, actorID, postID)
	if err != nil {
		return err
	}
	if created && authorID != actorID {
		actor := actorID
		post := postID
		in := notification.CreateInput{
			UserID:  authorID,
			ActorID: &actor,
			Type:    notification.TypePostLike,
			PostID:  &post,
		}
		if err := s.notifs.CreateTx(ctx, tx, in); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
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
