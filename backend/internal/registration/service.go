// Package registration creates an account and its first session atomically.
package registration

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"together/backend/internal/session"
	"together/backend/internal/user"
)

// Creator is the narrow registration dependency used by the HTTP server.
type Creator interface {
	Register(ctx context.Context, in user.CreateInput, tokenHash string, expiresAt time.Time) (*user.User, error)
}

type tx interface {
	user.DBTX
	session.DBTX
	Commit(context.Context) error
	Rollback(context.Context) error
}

type beginner interface {
	Begin(context.Context) (tx, error)
}

type userCreator interface {
	CreateTx(context.Context, user.DBTX, user.CreateInput) (*user.User, error)
}

type sessionCreator interface {
	CreateTx(context.Context, session.DBTX, string, string, time.Time) (*session.Session, error)
}

// Service coordinates account and first-session creation in one transaction.
type Service struct {
	db       beginner
	users    userCreator
	sessions sessionCreator
}

// New constructs the production registration service from PostgreSQL-backed
// repositories and their shared pool.
func New(pool *pgxpool.Pool, users userCreator, sessions sessionCreator) *Service {
	return &Service{db: poolBeginner{pool: pool}, users: users, sessions: sessions}
}

// Register creates the user and initial session atomically.
func (s *Service) Register(ctx context.Context, in user.CreateInput, tokenHash string, expiresAt time.Time) (*user.User, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	created, err := s.users.CreateTx(ctx, tx, in)
	if err != nil {
		return nil, err
	}
	if _, err := s.sessions.CreateTx(ctx, tx, created.ID, tokenHash, expiresAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return created, nil
}

type poolBeginner struct {
	pool *pgxpool.Pool
}

func (b poolBeginner) Begin(ctx context.Context) (tx, error) {
	return b.pool.Begin(ctx)
}

// Assert pgx transactions satisfy the shared transaction contract.
var _ tx = (pgx.Tx)(nil)
