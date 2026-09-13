package likeservice

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"together/backend/internal/like"
	"together/backend/internal/notification"
)

// fakeTx satisfies Tx. Exec/QueryRow/Query are unused (the fake repos ignore
// the executor); only Commit/Rollback are observed.
type fakeTx struct {
	committed  bool
	rolledBack bool
}

func (f *fakeTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (f *fakeTx) QueryRow(context.Context, string, ...any) pgx.Row { return nil }
func (f *fakeTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}
func (f *fakeTx) Commit(context.Context) error   { f.committed = true; return nil }
func (f *fakeTx) Rollback(context.Context) error { f.rolledBack = true; return nil }

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

type fakeLikeCreator struct {
	created bool
	err     error
}

func (f *fakeLikeCreator) LikeTx(_ context.Context, _ like.DBTX, _, _ string) (bool, error) {
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

func newService(tx *fakeTx, lc *fakeLikeCreator, nc *fakeNotifCreator) *Service {
	return &Service{db: &fakeBeginner{tx: tx}, likes: lc, notifs: nc}
}

func TestLikeCreatesNotification(t *testing.T) {
	tx := &fakeTx{}
	nc := &fakeNotifCreator{}
	if err := newService(tx, &fakeLikeCreator{created: true}, nc).Like(context.Background(), "actor", "post-1", "author"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nc.calls) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(nc.calls))
	}
	in := nc.calls[0]
	if in.UserID != "author" || in.ActorID == nil || *in.ActorID != "actor" ||
		in.Type != notification.TypePostLike || in.PostID == nil || *in.PostID != "post-1" {
		t.Fatalf("unexpected notification input: %+v", in)
	}
	if !tx.committed {
		t.Fatal("expected commit")
	}
}

func TestLikeDuplicateNoNotification(t *testing.T) {
	tx := &fakeTx{}
	nc := &fakeNotifCreator{}
	// ON CONFLICT DO NOTHING -> no new like -> created=false
	if err := newService(tx, &fakeLikeCreator{created: false}, nc).Like(context.Background(), "actor", "post-1", "author"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nc.calls) != 0 {
		t.Fatalf("expected no notification on duplicate like, got %d", len(nc.calls))
	}
	if !tx.committed {
		t.Fatal("expected commit even when no notification is created")
	}
}

func TestLikeSelfNoNotification(t *testing.T) {
	tx := &fakeTx{}
	nc := &fakeNotifCreator{}
	// actor == author: a new like is created, but no self-notification.
	if err := newService(tx, &fakeLikeCreator{created: true}, nc).Like(context.Background(), "me", "post-1", "me"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nc.calls) != 0 {
		t.Fatalf("self-like must not create a notification, got %d", len(nc.calls))
	}
	if !tx.committed {
		t.Fatal("expected commit")
	}
}

func TestLikeEdgeErrorRollsBack(t *testing.T) {
	tx := &fakeTx{}
	nc := &fakeNotifCreator{}
	if err := newService(tx, &fakeLikeCreator{err: errors.New("boom")}, nc).Like(context.Background(), "actor", "post-1", "author"); err == nil {
		t.Fatal("expected error")
	}
	if len(nc.calls) != 0 {
		t.Fatal("no notification should be created when the like fails")
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("expected rollback, committed=%v rolledBack=%v", tx.committed, tx.rolledBack)
	}
}

func TestLikeNotificationErrorRollsBack(t *testing.T) {
	tx := &fakeTx{}
	nc := &fakeNotifCreator{err: errors.New("boom")}
	if err := newService(tx, &fakeLikeCreator{created: true}, nc).Like(context.Background(), "actor", "post-1", "author"); err == nil {
		t.Fatal("expected error")
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("expected rollback, committed=%v rolledBack=%v", tx.committed, tx.rolledBack)
	}
}

func TestLikeBeginErrorSafe(t *testing.T) {
	s := &Service{db: &fakeBeginner{beginErr: errors.New("down")}, likes: &fakeLikeCreator{}, notifs: &fakeNotifCreator{}}
	if err := s.Like(context.Background(), "actor", "post-1", "author"); err == nil {
		t.Fatal("expected error when the transaction cannot begin")
	}
}
