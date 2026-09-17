package topic

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"together/backend/internal/post"
)

type fakeRow struct {
	id      string
	scanErr error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	if len(dest) > 0 {
		if p, ok := dest[0].(*string); ok {
			*p = r.id
		}
	}
	return nil
}

type fakeDB struct {
	querySQL  []string
	queryArgs [][]any
	rows      []fakeRow
	execSQL   []string
	execArgs  [][]any
	execErr   error
}

func (f *fakeDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	f.querySQL = append(f.querySQL, sql)
	f.queryArgs = append(f.queryArgs, args)
	row := fakeRow{}
	if len(f.querySQL) <= len(f.rows) {
		row = f.rows[len(f.querySQL)-1]
	}
	return row
}

func (f *fakeDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.execSQL = append(f.execSQL, sql)
	f.execArgs = append(f.execArgs, args)
	return pgconn.CommandTag{}, f.execErr
}

func TestCreatePostTopicsTxCreatesTopicAndLink(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{{id: "topic-new"}}}

	err := (&PostgresRepository{}).CreatePostTopicsTx(context.Background(), db, "post-1", []string{"travel"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(db.querySQL) != 1 || !strings.Contains(db.querySQL[0], "INSERT INTO topics") {
		t.Fatalf("expected topic insert query, got %v", db.querySQL)
	}
	if len(db.queryArgs[0]) != 1 || db.queryArgs[0][0] != "travel" {
		t.Fatalf("unexpected topic arguments: %v", db.queryArgs[0])
	}
	if len(db.execSQL) != 1 || len(db.execArgs[0]) != 2 || db.execArgs[0][0] != "post-1" || db.execArgs[0][1] != "topic-new" {
		t.Fatalf("unexpected post_topics link: sql=%v args=%v", db.execSQL, db.execArgs)
	}
}

func TestCreatePostTopicsTxReusesExistingTopic(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{{id: "topic-existing"}}}

	err := (&PostgresRepository{}).CreatePostTopicsTx(context.Background(), db, "post-2", []string{"travel"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(db.querySQL[0], "ON CONFLICT (slug)") {
		t.Fatalf("expected slug conflict handling, got %s", db.querySQL[0])
	}
	if db.execArgs[0][1] != "topic-existing" {
		t.Fatalf("expected reused topic ID in link, got %v", db.execArgs[0])
	}
}

func TestCreatePostTopicsTxDuplicateLinkIsSafe(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{{id: "topic-1"}}}

	if err := (&PostgresRepository{}).CreatePostTopicsTx(context.Background(), db, "post-1", []string{"travel"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(db.execSQL[0], "ON CONFLICT (post_id, topic_id) DO NOTHING") {
		t.Fatalf("expected duplicate-link protection, got %s", db.execSQL[0])
	}
}

func TestCreatePostTopicsTxPropagatesScanError(t *testing.T) {
	scanErr := errors.New("scan failed")
	db := &fakeDB{rows: []fakeRow{{scanErr: scanErr}}}

	err := (&PostgresRepository{}).CreatePostTopicsTx(context.Background(), db, "post-1", []string{"travel"})
	if !errors.Is(err, scanErr) {
		t.Fatalf("expected scan error, got %v", err)
	}
	if len(db.execSQL) != 0 {
		t.Fatalf("link must not run after scan error: %v", db.execSQL)
	}
}

func TestCreatePostTopicsTxPropagatesExecError(t *testing.T) {
	execErr := errors.New("exec failed")
	db := &fakeDB{rows: []fakeRow{{id: "topic-1"}}, execErr: execErr}

	err := (&PostgresRepository{}).CreatePostTopicsTx(context.Background(), db, "post-1", []string{"travel"})
	if !errors.Is(err, execErr) {
		t.Fatalf("expected exec error, got %v", err)
	}
}

func TestCreatePostTopicsTxEmptySlugsExecutesNoSQL(t *testing.T) {
	db := &fakeDB{}

	if err := (&PostgresRepository{}).CreatePostTopicsTx(context.Background(), db, "post-1", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(db.querySQL) != 0 || len(db.execSQL) != 0 {
		t.Fatalf("empty slug list must not execute SQL: queries=%v execs=%v", db.querySQL, db.execSQL)
	}
}

func TestSearchTopicsQueryIsParameterizedAndVisibilitySafe(t *testing.T) {
	if !strings.Contains(searchTopicsQuery, "position($2 in t.slug)") || !strings.Contains(searchTopicsQuery, "LIMIT $3") {
		t.Fatalf("search query must parameterize its input and limit: %s", searchTopicsQuery)
	}
	if !strings.Contains(searchTopicsQuery, post.SQLFollowsAuthor) || !strings.Contains(searchTopicsQuery, post.SQLNotBlocked) {
		t.Fatalf("search query must reuse visibility and block predicates: %s", searchTopicsQuery)
	}
}
