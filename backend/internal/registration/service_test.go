package registration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"together/backend/internal/session"
	"together/backend/internal/user"
)

type fakeRow struct{}

func (fakeRow) Scan(...any) error { return nil }

type fakeTx struct {
	committed  bool
	rolledBack bool
	commitErr  error
}

func (t *fakeTx) QueryRow(context.Context, string, ...any) pgx.Row { return fakeRow{} }
func (t *fakeTx) Commit(context.Context) error {
	t.committed = true
	return t.commitErr
}
func (t *fakeTx) Rollback(context.Context) error {
	t.rolledBack = true
	return nil
}

type fakeBeginner struct {
	tx  *fakeTx
	err error
}

func (b *fakeBeginner) Begin(context.Context) (tx, error) { return b.tx, b.err }

type fakeUsers struct {
	created bool
	err     error
}

func (f *fakeUsers) CreateTx(_ context.Context, _ user.DBTX, _ user.CreateInput) (*user.User, error) {
	f.created = true
	if f.err != nil {
		return nil, f.err
	}
	return &user.User{ID: "11111111-1111-1111-1111-111111111111"}, nil
}

type fakeSessions struct {
	created bool
	err     error
}

func (f *fakeSessions) CreateTx(_ context.Context, _ session.DBTX, _ string, _ string, _ time.Time) (*session.Session, error) {
	f.created = true
	if f.err != nil {
		return nil, f.err
	}
	return &session.Session{}, nil
}

func TestRegisterCommitsUserAndSessionTogether(t *testing.T) {
	tx := &fakeTx{}
	users := &fakeUsers{}
	sessions := &fakeSessions{}
	svc := &Service{db: &fakeBeginner{tx: tx}, users: users, sessions: sessions}

	if _, err := svc.Register(context.Background(), user.CreateInput{}, "token-hash", time.Now()); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if !users.created || !sessions.created || !tx.committed {
		t.Fatalf("registration did not complete atomically: users=%v sessions=%v committed=%v", users.created, sessions.created, tx.committed)
	}
}

func TestRegisterRollsBackWhenSessionCreationFails(t *testing.T) {
	tx := &fakeTx{}
	users := &fakeUsers{}
	sessions := &fakeSessions{err: errors.New("session unavailable")}
	svc := &Service{db: &fakeBeginner{tx: tx}, users: users, sessions: sessions}

	if _, err := svc.Register(context.Background(), user.CreateInput{}, "token-hash", time.Now()); err == nil {
		t.Fatal("expected session creation error")
	}
	if !users.created || !sessions.created || !tx.rolledBack || tx.committed {
		t.Fatalf("failed registration left a partial transaction: users=%v sessions=%v rollback=%v commit=%v", users.created, sessions.created, tx.rolledBack, tx.committed)
	}
}
