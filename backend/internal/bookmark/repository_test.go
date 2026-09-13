package bookmark

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeRow returns a preset bool from Scan.
type fakeRow struct {
	exists  bool
	scanErr error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	if len(dest) > 0 {
		if p, ok := dest[0].(*bool); ok {
			*p = r.exists
		}
	}
	return nil
}

// fakeDB records the last Exec/QueryRow and returns preset results.
type fakeDB struct {
	lastSQL  string
	lastArgs []any
	execErr  error
	queryErr error
	row      fakeRow
}

func (f *fakeDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.lastSQL = sql
	f.lastArgs = args
	return pgconn.CommandTag{}, f.execErr
}

func (f *fakeDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	f.lastSQL = sql
	f.lastArgs = args
	return f.row
}

func (f *fakeDB) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	f.lastSQL = sql
	f.lastArgs = args
	return nil, f.queryErr
}

func newRepo(db DBTX) *PostgresRepository { return &PostgresRepository{db: db} }

func TestSave(t *testing.T) {
	db := &fakeDB{}
	if err := newRepo(db).Save(context.Background(), "u", "p"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(db.lastSQL, "INSERT INTO post_bookmarks") || !strings.Contains(db.lastSQL, "ON CONFLICT DO NOTHING") {
		t.Fatalf("unexpected save query: %s", db.lastSQL)
	}
	if len(db.lastArgs) != 2 || db.lastArgs[0] != "u" || db.lastArgs[1] != "p" {
		t.Fatalf("unexpected args: %v", db.lastArgs)
	}
}

func TestRemove(t *testing.T) {
	db := &fakeDB{}
	if err := newRepo(db).Remove(context.Background(), "u", "p"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(db.lastSQL, "DELETE FROM post_bookmarks") {
		t.Fatalf("unexpected remove query: %s", db.lastSQL)
	}
	if len(db.lastArgs) != 2 || db.lastArgs[0] != "u" || db.lastArgs[1] != "p" {
		t.Fatalf("unexpected args: %v", db.lastArgs)
	}
}

func TestIsSavedTrue(t *testing.T) {
	db := &fakeDB{row: fakeRow{exists: true}}
	got, err := newRepo(db).IsSaved(context.Background(), "u", "p")
	if err != nil || !got {
		t.Fatalf("expected true, got %v err=%v", got, err)
	}
}

func TestIsSavedFalse(t *testing.T) {
	db := &fakeDB{row: fakeRow{exists: false}}
	got, err := newRepo(db).IsSaved(context.Background(), "u", "p")
	if err != nil || got {
		t.Fatalf("expected false, got %v err=%v", got, err)
	}
}

func TestListSavedPostIDsEmpty(t *testing.T) {
	db := &fakeDB{}
	m, err := newRepo(db).ListSavedPostIDs(context.Background(), "u", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m) != 0 {
		t.Fatalf("expected empty map, got %v", m)
	}
	if db.lastSQL != "" {
		t.Fatalf("empty postIDs must not run SQL, ran: %s", db.lastSQL)
	}
}
