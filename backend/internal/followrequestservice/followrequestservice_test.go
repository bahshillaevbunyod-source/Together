package followrequestservice

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"together/backend/internal/follow"
	"together/backend/internal/followrequest"
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

// fakeRequestStore satisfies RequestStore (CreateTx + DeleteTx).
type fakeRequestStore struct {
	// delete side
	existed    bool
	deleteErr  error
	deleteFrom string
	deleteTo   string
	deletes    int
	// create side
	created    bool
	createErr  error
	createFrom string
	createTo   string
	creates    int
}

func (f *fakeRequestStore) DeleteTx(_ context.Context, _ followrequest.DBTX, requesterID, targetID string) (bool, error) {
	f.deletes++
	f.deleteFrom = requesterID
	f.deleteTo = targetID
	return f.existed, f.deleteErr
}

func (f *fakeRequestStore) CreateTx(_ context.Context, _ followrequest.DBTX, requesterID, targetID string) (bool, error) {
	f.creates++
	f.createFrom = requesterID
	f.createTo = targetID
	return f.created, f.createErr
}

type fakeFollowCreator struct {
	created  bool
	err      error
	calls    int
	lastFrom string
	lastTo   string
}

func (f *fakeFollowCreator) FollowTx(_ context.Context, _ follow.DBTX, followerID, followingID string) (bool, error) {
	f.calls++
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

func newService(tx *fakeTx, rd *fakeRequestStore, fc *fakeFollowCreator, nc *fakeNotifCreator) *Service {
	return &Service{db: &fakeBeginner{tx: tx}, requests: rd, follows: fc, notifs: nc}
}

func TestAcceptDeletesRequestCreatesEdgeAndNotifies(t *testing.T) {
	tx := &fakeTx{}
	rd := &fakeRequestStore{existed: true}
	fc := &fakeFollowCreator{created: true}
	nc := &fakeNotifCreator{}

	if err := newService(tx, rd, fc, nc).Accept(context.Background(), "requester", "target"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Request deleted for the exact pair.
	if rd.deletes != 1 || rd.deleteFrom != "requester" || rd.deleteTo != "target" {
		t.Fatalf("unexpected request delete: %+v", rd)
	}
	// Accepted edge is requester -> target (requester follows target).
	if fc.calls != 1 || fc.lastFrom != "requester" || fc.lastTo != "target" {
		t.Fatalf("unexpected follow edge: %+v", fc)
	}
	// Notification goes to the target, actor is the requester, type is follow.
	if len(nc.calls) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(nc.calls))
	}
	in := nc.calls[0]
	if in.UserID != "target" || in.ActorID == nil || *in.ActorID != "requester" || in.Type != notification.TypeFollow {
		t.Fatalf("unexpected notification input: %+v", in)
	}
	if !tx.committed {
		t.Fatal("expected commit")
	}
}

func TestAcceptNoPendingRequestReturnsErrNoRequest(t *testing.T) {
	tx := &fakeTx{}
	rd := &fakeRequestStore{existed: false} // nothing to accept
	fc := &fakeFollowCreator{}
	nc := &fakeNotifCreator{}

	err := newService(tx, rd, fc, nc).Accept(context.Background(), "requester", "target")
	if !errors.Is(err, ErrNoRequest) {
		t.Fatalf("expected ErrNoRequest, got %v", err)
	}
	// No edge, no notification, no commit — nothing was written.
	if fc.calls != 0 {
		t.Fatal("no follow edge must be created when there is no pending request")
	}
	if len(nc.calls) != 0 {
		t.Fatal("no notification must be created when there is no pending request")
	}
	if tx.committed {
		t.Fatal("transaction must not commit when there is no pending request")
	}
	if !tx.rolledBack {
		t.Fatal("transaction must roll back when there is no pending request")
	}
}

func TestAcceptDuplicateEdgeStillNoDoubleNotification(t *testing.T) {
	tx := &fakeTx{}
	rd := &fakeRequestStore{existed: true}
	fc := &fakeFollowCreator{created: false} // edge already existed
	nc := &fakeNotifCreator{}

	if err := newService(tx, rd, fc, nc).Accept(context.Background(), "requester", "target"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nc.calls) != 0 {
		t.Fatalf("expected no notification when edge already existed, got %d", len(nc.calls))
	}
	if !tx.committed {
		t.Fatal("expected commit even when no new edge was created")
	}
}

func TestAcceptEdgeErrorRollsBack(t *testing.T) {
	tx := &fakeTx{}
	rd := &fakeRequestStore{existed: true}
	fc := &fakeFollowCreator{err: errors.New("boom")}
	nc := &fakeNotifCreator{}

	if err := newService(tx, rd, fc, nc).Accept(context.Background(), "requester", "target"); err == nil {
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

func TestAcceptNotificationErrorRollsBack(t *testing.T) {
	tx := &fakeTx{}
	rd := &fakeRequestStore{existed: true}
	fc := &fakeFollowCreator{created: true}
	nc := &fakeNotifCreator{err: errors.New("boom")}

	if err := newService(tx, rd, fc, nc).Accept(context.Background(), "requester", "target"); err == nil {
		t.Fatal("expected error")
	}
	if tx.committed {
		t.Fatal("transaction must not commit when the notification fails")
	}
	if !tx.rolledBack {
		t.Fatal("transaction must roll back when the notification fails")
	}
}

func TestAcceptBeginErrorSafe(t *testing.T) {
	s := &Service{
		db:       &fakeBeginner{beginErr: errors.New("down")},
		requests: &fakeRequestStore{},
		follows:  &fakeFollowCreator{},
		notifs:   &fakeNotifCreator{},
	}
	if err := s.Accept(context.Background(), "requester", "target"); err == nil {
		t.Fatal("expected error when the transaction cannot begin")
	}
}

func TestRequestCreatesPendingAndNotifiesButNoFollowEdge(t *testing.T) {
	tx := &fakeTx{}
	rd := &fakeRequestStore{created: true}
	fc := &fakeFollowCreator{}
	nc := &fakeNotifCreator{}

	created, err := newService(tx, rd, fc, nc).Request(context.Background(), "requester", "target")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Fatal("expected created=true for a new request")
	}
	// Request written for the exact pair.
	if rd.creates != 1 || rd.createFrom != "requester" || rd.createTo != "target" {
		t.Fatalf("unexpected request create: %+v", rd)
	}
	// CRITICAL: a pending request must NOT insert a follows edge.
	if fc.calls != 0 {
		t.Fatal("Request must never create a follows edge")
	}
	// follow_request notification to the target, actor is the requester.
	if len(nc.calls) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(nc.calls))
	}
	in := nc.calls[0]
	if in.UserID != "target" || in.ActorID == nil || *in.ActorID != "requester" || in.Type != notification.TypeFollowRequest {
		t.Fatalf("unexpected notification input: %+v", in)
	}
	if !tx.committed {
		t.Fatal("expected commit")
	}
}

func TestRequestDuplicateNoNotification(t *testing.T) {
	tx := &fakeTx{}
	rd := &fakeRequestStore{created: false} // ON CONFLICT DO NOTHING -> no new row
	fc := &fakeFollowCreator{}
	nc := &fakeNotifCreator{}

	created, err := newService(tx, rd, fc, nc).Request(context.Background(), "requester", "target")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created {
		t.Fatal("expected created=false for a duplicate request")
	}
	if len(nc.calls) != 0 {
		t.Fatalf("expected no notification on duplicate request, got %d", len(nc.calls))
	}
	if fc.calls != 0 {
		t.Fatal("Request must never create a follows edge")
	}
	if !tx.committed {
		t.Fatal("expected commit even when no new request row was created")
	}
}

func TestRequestNotificationErrorRollsBack(t *testing.T) {
	tx := &fakeTx{}
	rd := &fakeRequestStore{created: true}
	fc := &fakeFollowCreator{}
	nc := &fakeNotifCreator{err: errors.New("boom")}

	if _, err := newService(tx, rd, fc, nc).Request(context.Background(), "requester", "target"); err == nil {
		t.Fatal("expected error")
	}
	if tx.committed {
		t.Fatal("transaction must not commit when the notification fails")
	}
	if !tx.rolledBack {
		t.Fatal("transaction must roll back when the notification fails")
	}
}
