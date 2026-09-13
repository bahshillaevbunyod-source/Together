// Package followservice provides the transactional follow use-case: creating a
// follow edge and, when the edge is new, its "follow" notification atomically.
package followservice

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"together/backend/internal/follow"
	"together/backend/internal/notification"
)

// Tx is a transaction usable as a query executor by both repositories.
// notification.DBTX is the wider executor (Exec/QueryRow/Query) and also covers
// follow.DBTX's single Exec method.
type Tx interface {
	notification.DBTX
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// Beginner opens transactions.
type Beginner interface {
	Begin(ctx context.Context) (Tx, error)
}

// FollowCreator inserts a follow edge within a transaction, reporting whether a
// new edge was created.
type FollowCreator interface {
	FollowTx(ctx context.Context, q follow.DBTX, followerID, followingID string) (bool, error)
}

// NotificationCreator inserts a notification within a transaction.
type NotificationCreator interface {
	CreateTx(ctx context.Context, q notification.DBTX, in notification.CreateInput) error
}

// Service follows a user and records the follow notification atomically.
type Service struct {
	db      Beginner
	follows FollowCreator
	notifs  NotificationCreator
}

// New builds a Service. The pool is wrapped so its transactions satisfy Tx.
func New(pool *pgxpool.Pool, follows FollowCreator, notifs NotificationCreator) *Service {
	return &Service{db: poolBeginner{pool: pool}, follows: follows, notifs: notifs}
}

// Follow makes followerID follow followingID and, only when the follow edge is
// newly created, records a "follow" notification for the followed user — both
// in one transaction. A duplicate follow creates no duplicate notification. The
// caller is responsible for rejecting self-follows and blocked pairs before
// calling this. On any error the transaction is rolled back.
func (s *Service) Follow(ctx context.Context, followerID, followingID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // no-op after a successful Commit

	created, err := s.follows.FollowTx(ctx, tx, followerID, followingID)
	if err != nil {
		return err
	}
	if created {
		actor := followerID
		in := notification.CreateInput{
			UserID:  followingID,
			ActorID: &actor,
			Type:    notification.TypeFollow,
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
