// Package followrequestservice provides the transactional accept use-case for a
// pending follow request: verifying and deleting the pending request, inserting
// the canonical accepted edge into `follows`, and recording the follow
// notification — all atomically, so no half-state (a request that is neither
// pending nor accepted, or an edge with no notification) can ever be observed.
package followrequestservice

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"together/backend/internal/follow"
	"together/backend/internal/followrequest"
	"together/backend/internal/notification"
)

// ErrNoRequest is returned by Accept when there is no pending request to accept
// (already accepted, declined, cancelled, or never existed). Nothing is written.
var ErrNoRequest = errors.New("no pending follow request")

// Tx is a transaction usable as a query executor by every repository involved.
// notification.DBTX is the widest executor (Exec/QueryRow/Query) and also
// satisfies follow.DBTX's and followrequest.DBTX's single Exec method.
type Tx interface {
	notification.DBTX
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// Beginner opens transactions.
type Beginner interface {
	Begin(ctx context.Context) (Tx, error)
}

// RequestDeleter deletes a pending request within a transaction, reporting
// whether a row existed.
type RequestDeleter interface {
	DeleteTx(ctx context.Context, q followrequest.DBTX, requesterID, targetID string) (bool, error)
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

// Service accepts a pending follow request atomically.
type Service struct {
	db       Beginner
	requests RequestDeleter
	follows  FollowCreator
	notifs   NotificationCreator
}

// New builds a Service. The pool is wrapped so its transactions satisfy Tx.
func New(pool *pgxpool.Pool, requests RequestDeleter, follows FollowCreator, notifs NotificationCreator) *Service {
	return &Service{db: poolBeginner{pool: pool}, requests: requests, follows: follows, notifs: notifs}
}

// Accept approves the pending request from requesterID to targetID. In one
// transaction it deletes the pending request, inserts the accepted follow edge
// (requester follows target — identical to a normal follow), and records the
// "follow" notification for the target when the edge is newly created. If there
// is no pending request, it returns ErrNoRequest and writes nothing. On any
// error the transaction is rolled back, so the request/edge/notification are
// always consistent.
func (s *Service) Accept(ctx context.Context, requesterID, targetID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // no-op after a successful Commit

	existed, err := s.requests.DeleteTx(ctx, tx, requesterID, targetID)
	if err != nil {
		return err
	}
	if !existed {
		return ErrNoRequest
	}

	// The accepted edge is the canonical follow: requester becomes a follower
	// of target. This is the ONLY place the follow graph is touched on accept.
	created, err := s.follows.FollowTx(ctx, tx, requesterID, targetID)
	if err != nil {
		return err
	}
	if created {
		actor := requesterID
		in := notification.CreateInput{
			UserID:  targetID,
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
