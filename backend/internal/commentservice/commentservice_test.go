package commentservice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"together/backend/internal/comment"
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

type fakeCommentCreator struct {
	comment *comment.Comment
	err     error
}

func (f *fakeCommentCreator) CreateTx(_ context.Context, _ comment.DBTX, in comment.CreateInput) (*comment.Comment, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.comment != nil {
		return f.comment, nil
	}
	return &comment.Comment{ID: "comment-1", PostID: in.PostID, AuthorID: in.AuthorID, Content: in.Content, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
}

type fakeNotifCreator struct {
	err   error
	calls []notification.CreateInput
}

func (f *fakeNotifCreator) CreateTx(_ context.Context, _ notification.DBTX, in notification.CreateInput) error {
	f.calls = append(f.calls, in)
	return f.err
}

func newService(tx *fakeTx, cc *fakeCommentCreator, nc *fakeNotifCreator) *Service {
	return &Service{db: &fakeBeginner{tx: tx}, comments: cc, notifs: nc}
}

func in(author string) comment.CreateInput {
	return comment.CreateInput{PostID: "post-1", AuthorID: author, Content: "hi"}
}

func TestCreateCommentNotifies(t *testing.T) {
	tx := &fakeTx{}
	nc := &fakeNotifCreator{}
	c, err := newService(tx, &fakeCommentCreator{}, nc).CreateComment(context.Background(), in("actor"), "author")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil || c.ID != "comment-1" {
		t.Fatalf("expected the created comment, got %+v", c)
	}
	if len(nc.calls) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(nc.calls))
	}
	n := nc.calls[0]
	if n.UserID != "author" || n.ActorID == nil || *n.ActorID != "actor" ||
		n.Type != notification.TypePostComment || n.PostID == nil || *n.PostID != "post-1" ||
		n.CommentID == nil || *n.CommentID != "comment-1" {
		t.Fatalf("unexpected notification input: %+v", n)
	}
	if !tx.committed {
		t.Fatal("expected commit")
	}
}

func TestCreateCommentSelfNoNotification(t *testing.T) {
	tx := &fakeTx{}
	nc := &fakeNotifCreator{}
	// commenter == post author -> no self-notification.
	if _, err := newService(tx, &fakeCommentCreator{}, nc).CreateComment(context.Background(), in("me"), "me"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nc.calls) != 0 {
		t.Fatalf("self-comment must not create a notification, got %d", len(nc.calls))
	}
	if !tx.committed {
		t.Fatal("expected commit")
	}
}

func TestCreateCommentErrorRollsBack(t *testing.T) {
	tx := &fakeTx{}
	nc := &fakeNotifCreator{}
	if _, err := newService(tx, &fakeCommentCreator{err: errors.New("boom")}, nc).CreateComment(context.Background(), in("actor"), "author"); err == nil {
		t.Fatal("expected error")
	}
	if len(nc.calls) != 0 {
		t.Fatal("no notification should be created when the comment fails")
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("expected rollback, committed=%v rolledBack=%v", tx.committed, tx.rolledBack)
	}
}

func TestCreateCommentNotificationErrorRollsBack(t *testing.T) {
	tx := &fakeTx{}
	nc := &fakeNotifCreator{err: errors.New("boom")}
	if _, err := newService(tx, &fakeCommentCreator{}, nc).CreateComment(context.Background(), in("actor"), "author"); err == nil {
		t.Fatal("expected error")
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("expected rollback, committed=%v rolledBack=%v", tx.committed, tx.rolledBack)
	}
}

func TestCreateCommentBeginErrorSafe(t *testing.T) {
	s := &Service{db: &fakeBeginner{beginErr: errors.New("down")}, comments: &fakeCommentCreator{}, notifs: &fakeNotifCreator{}}
	if _, err := s.CreateComment(context.Background(), in("actor"), "author"); err == nil {
		t.Fatal("expected error when the transaction cannot begin")
	}
}
