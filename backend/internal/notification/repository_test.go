package notification

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeRow struct{ scan func(dest ...any) error }

func (r fakeRow) Scan(dest ...any) error { return r.scan(dest...) }

type fakeDB struct {
	lastSQL  string
	lastArgs []any
	row      fakeRow
	execTag  pgconn.CommandTag
	execErr  error
}

func (f *fakeDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.lastSQL = sql
	f.lastArgs = args
	return f.execTag, f.execErr
}

func (f *fakeDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	f.lastSQL = sql
	f.lastArgs = args
	return f.row
}

func (f *fakeDB) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	f.lastSQL = sql
	f.lastArgs = args
	return nil, nil
}

func newRepo(db DBTX) *PostgresRepository { return &PostgresRepository{db: db} }

func strPtr(s string) *string { return &s }

func TestCreate(t *testing.T) {
	// scan fills the RETURNING columns.
	db := &fakeDB{row: fakeRow{scan: func(dest ...any) error {
		for _, d := range dest {
			switch p := d.(type) {
			case *string:
				*p = "v"
			case **string:
				*p = nil
			case *time.Time:
				*p = time.Now()
			case **time.Time:
				*p = nil
			}
		}
		return nil
	}}}

	actor := strPtr("actor-1")
	got, err := newRepo(db).Create(context.Background(), CreateInput{
		UserID: "u1", ActorID: actor, Type: TypeFollow, PostID: nil, CommentID: nil,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a notification")
	}
	if !strings.Contains(db.lastSQL, "INSERT INTO notifications") || !strings.Contains(db.lastSQL, "RETURNING") {
		t.Fatalf("unexpected query: %s", db.lastSQL)
	}
	if len(db.lastArgs) != 5 || db.lastArgs[0] != "u1" || db.lastArgs[2] != TypeFollow {
		t.Fatalf("unexpected args: %v", db.lastArgs)
	}
	if db.lastArgs[1] != actor {
		t.Fatalf("actor arg not passed through: %v", db.lastArgs[1])
	}
}

func TestMarkReadFound(t *testing.T) {
	db := &fakeDB{execTag: pgconn.NewCommandTag("UPDATE 1")}
	ok, err := newRepo(db).MarkRead(context.Background(), "u1", "n1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected found=true when a row matched")
	}
	if !strings.Contains(db.lastSQL, "UPDATE notifications") || !strings.Contains(db.lastSQL, "COALESCE(read_at") {
		t.Fatalf("unexpected query: %s", db.lastSQL)
	}
	if len(db.lastArgs) != 2 || db.lastArgs[0] != "n1" || db.lastArgs[1] != "u1" {
		t.Fatalf("unexpected args: %v", db.lastArgs)
	}
}

func TestMarkReadNotFound(t *testing.T) {
	db := &fakeDB{execTag: pgconn.NewCommandTag("UPDATE 0")}
	ok, err := newRepo(db).MarkRead(context.Background(), "u1", "n1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected found=false when no row matched")
	}
}

func TestMarkAllRead(t *testing.T) {
	db := &fakeDB{execTag: pgconn.NewCommandTag("UPDATE 4")}
	if err := newRepo(db).MarkAllRead(context.Background(), "u1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(db.lastSQL, "UPDATE notifications") || !strings.Contains(db.lastSQL, "read_at IS NULL") {
		t.Fatalf("unexpected query: %s", db.lastSQL)
	}
	if len(db.lastArgs) != 1 || db.lastArgs[0] != "u1" {
		t.Fatalf("unexpected args: %v", db.lastArgs)
	}
}

func TestCountUnread(t *testing.T) {
	db := &fakeDB{row: fakeRow{scan: func(dest ...any) error {
		if p, ok := dest[0].(*int64); ok {
			*p = 3
		}
		return nil
	}}}

	got, err := newRepo(db).CountUnread(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
	if !strings.Contains(db.lastSQL, "count(*)") || !strings.Contains(db.lastSQL, "read_at IS NULL") {
		t.Fatalf("unexpected query: %s", db.lastSQL)
	}
	if len(db.lastArgs) != 1 || db.lastArgs[0] != "u1" {
		t.Fatalf("unexpected args: %v", db.lastArgs)
	}
}
