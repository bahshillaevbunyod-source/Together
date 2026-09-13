// Package commentservice provides the transactional comment use-case: creating
// a comment and, when the commenter is not the post's author, its "post_comment"
// notification atomically.
package commentservice

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"together/backend/internal/comment"
	"together/backend/internal/notification"
)

// Tx is a transaction usable as a query executor by both repositories.
// notification.DBTX is the wider executor (Exec/QueryRow/Query) and also covers
// comment.DBTX's Exec/QueryRow methods.
type Tx interface {
	notification.DBTX
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// Beginner opens transactions.
type Beginner interface {
	Begin(ctx context.Context) (Tx, error)
}

// CommentCreator inserts a comment within a transaction and returns it.
type CommentCreator interface {
	CreateTx(ctx context.Context, q comment.DBTX, in comment.CreateInput) (*comment.Comment, error)
}

// NotificationCreator inserts a notification within a transaction.
type NotificationCreator interface {
	CreateTx(ctx context.Context, q notification.DBTX, in notification.CreateInput) error
}

// Service creates a comment and records the comment notification atomically.
type Service struct {
	db       Beginner
	comments CommentCreator
	notifs   NotificationCreator
}

// New builds a Service. The pool is wrapped so its transactions satisfy Tx.
func New(pool *pgxpool.Pool, comments CommentCreator, notifs NotificationCreator) *Service {
	return &Service{db: poolBeginner{pool: pool}, comments: comments, notifs: notifs}
}

// CreateComment inserts the comment (in.AuthorID is the commenter) and, only
// when the commenter is not the post's author, records a "post_comment"
// notification for postAuthorID — both in one transaction. A self-comment
// creates no notification. The caller is responsible for enforcing post
// visibility/block access before calling this. On any error the transaction is
// rolled back and no comment persists.
func (s *Service) CreateComment(ctx context.Context, in comment.CreateInput, postAuthorID string) (*comment.Comment, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) // no-op after a successful Commit

	c, err := s.comments.CreateTx(ctx, tx, in)
	if err != nil {
		return nil, err
	}
	if in.AuthorID != postAuthorID {
		actor := in.AuthorID
		post := in.PostID
		commentID := c.ID
		n := notification.CreateInput{
			UserID:    postAuthorID,
			ActorID:   &actor,
			Type:      notification.TypePostComment,
			PostID:    &post,
			CommentID: &commentID,
		}
		if err := s.notifs.CreateTx(ctx, tx, n); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return c, nil
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
