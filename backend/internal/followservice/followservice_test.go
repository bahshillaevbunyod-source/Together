package followservice

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"together/backend/internal/follow"
	"together/backend/internal/notification"
)

// fakeTx satisfies Tx. Exec/QueryRow are unused (the fake repos ignore the
// executor); only Commit/Rollback are observed.
type fakeTx struct {
	committed  bool
	rolledBack bool
	commitErr  error
}

func (f *fakeTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (f *fakeTx) QueryRow(context.Context, string, ...any) pgx.Row { return nil }
func (f *fakeTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}
func (f *fakeTx) Commit(context.Context) error {
	f.committed = true
	return f.commitErr
}
func (f *fakeTx) Rollback(context.Context) error {
	f.rolledBack = true
	return nil
}

type fakeBeginner struct {
	tx       *fakeTx
	beginErr error
}

func (b *fakeBeginner) Begin(context.Context) (Tx, error) {
	if b.beginErr != nil {
		return nil, b.beginErr
	}
	return b.tx, nil
}

type fakeFollowCreator struct {
	created   bool
	err       error
	lastFrom  string
	lastTo    string
	callCount int
}

func (f *fakeFollowCreator) FollowTx(_ context.Context, _ follow.DBTX, followerID, followingID string) (bool, error) {
	f.callCount++
	f.lastFrom = followerID
	f.lastTo = followingID
	return f.created, f.err
}

type fakeNotifCreator struct {
	err   error
	calls []notification.CreateInput
}

func (f *fakeNotifCreator) CreateTx(_ context.Context, _ notification.DBTX, in notification.CreateInput) error {
	f.calls = append(f.calls, in)
	return f.err
}

func newService(tx *fakeTx, fc *fakeFollowCreator, nc *fakeNotifCreator) *Service {
	return &Service{db: &fakeBeginner{tx: tx}, follows: fc, notifs: nc}
}

func TestFollowCreatesNotification(t *testing.T) {
	tx := &fakeTx{}
	fc := &fakeFollowCreator{created: true}
	nc := &fakeNotifCreator{}
	if err := newService(tx, fc, nc).Follow(context.Background(), "actor", "target"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nc.calls) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(nc.calls))
	}
	in := nc.calls[0]
	if in.UserID != "target" || in.ActorID == nil || *in.ActorID != "actor" || in.Type != notification.TypeFollow {
		t.Fatalf("unexpected notification input: %+v", in)
	}
	if !tx.committed {
		t.Fatalf("expected commit, got committed=%v", tx.committed)
	}
}

func TestFollowDuplicateNoNotification(t *testing.T) {
	tx := &fakeTx{}
	fc := &fakeFollowCreator{created: false} // ON CONFLICT DO NOTHING -> no new edge
	nc := &fakeNotifCreator{}
	if err := newService(tx, fc, nc).Follow(context.Background(), "actor", "target"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nc.calls) != 0 {
		t.Fatalf("expected no notification on duplicate follow, got %d", len(nc.calls))
	}
	if !tx.committed {
		t.Fatal("expected commit even when no notification is created")
	}
}

func TestFollowEdgeErrorRollsBack(t *testing.T) {
	tx := &fakeTx{}
	fc := &fakeFollowCreator{err: errors.New("boom")}
	nc := &fakeNotifCreator{}
	if err := newService(tx, fc, nc).Follow(context.Background(), "actor", "target"); err == nil {
		t.Fatal("expected error")
	}
	if len(nc.calls) != 0 {
		t.Fatal("no notification should be created when the follow fails")
	}
	if tx.committed {
		t.Fatal("transaction must not commit on follow error")
	}
	if !tx.rolledBack {
		t.Fatal("transaction must roll back on follow error")
	}
}

func TestFollowNotificationErrorRollsBack(t *testing.T) {
	tx := &fakeTx{}
	fc := &fakeFollowCreator{created: true}
	nc := &fakeNotifCreator{err: errors.New("boom")}
	if err := newService(tx, fc, nc).Follow(context.Background(), "actor", "target"); err == nil {
		t.Fatal("expected error")
	}
	if tx.committed {
		t.Fatal("transaction must not commit when the notification fails")
	}
	if !tx.rolledBack {
		t.Fatal("transaction must roll back when the notification fails")
	}
}

func TestFollowBeginErrorSafe(t *testing.T) {
	s := &Service{db: &fakeBeginner{beginErr: errors.New("down")}, follows: &fakeFollowCreator{}, notifs: &fakeNotifCreator{}}
	if err := s.Follow(context.Background(), "actor", "target"); err == nil {
		t.Fatal("expected error when the transaction cannot begin")
	}
}
