package postservice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"together/backend/internal/media"
	"together/backend/internal/post"
)

// fakeTx implements Tx. The service only calls Commit/Rollback directly; Exec /
// QueryRow exist to satisfy the interface (the fake repos ignore the executor).
type fakeTx struct {
	committed  bool
	rolledBack bool
	commitErr  error
}

func (f *fakeTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (f *fakeTx) QueryRow(context.Context, string, ...any) pgx.Row { return nil }
func (f *fakeTx) Commit(context.Context) error {
	f.committed = true
	return f.commitErr
}
func (f *fakeTx) Rollback(context.Context) error {
	if f.committed {
		return pgx.ErrTxClosed // no-op after commit, like real pgx
	}
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

type fakePostCreator struct {
	post *post.Post
	err  error
}

func (f *fakePostCreator) CreateTx(_ context.Context, _ post.DBTX, in post.CreateInput) (*post.Post, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.post != nil {
		return f.post, nil
	}
	return &post.Post{ID: "post-1", AuthorID: in.AuthorID, Content: in.Content, Visibility: in.Visibility, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
}

type fakeMediaCreator struct {
	err      error
	received []media.CreateInput
}

func (f *fakeMediaCreator) CreateManyTx(_ context.Context, _ media.DBTX, items []media.CreateInput) error {
	f.received = items
	return f.err
}

func newService(tx *fakeTx, posts *fakePostCreator, mediaC *fakeMediaCreator) *Service {
	return &Service{db: &fakeBeginner{tx: tx}, posts: posts, media: mediaC}
}

func postInput() post.CreateInput {
	c := "hi"
	return post.CreateInput{AuthorID: "me-id", Content: &c, Visibility: "public"}
}

func TestCreatePostOnlyCommits(t *testing.T) {
	tx := &fakeTx{}
	p, err := newService(tx, &fakePostCreator{}, &fakeMediaCreator{}).
		CreatePostWithMedia(context.Background(), postInput(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil || !tx.committed || tx.rolledBack {
		t.Fatalf("expected commit, got committed=%v rolledBack=%v", tx.committed, tx.rolledBack)
	}
}

func TestCreatePostWithMediaCommits(t *testing.T) {
	tx := &fakeTx{}
	mediaC := &fakeMediaCreator{}
	items := []media.CreateInput{
		{Type: media.TypeImage, StorageKey: "a", MimeType: "image/png", SizeBytes: 1, SortOrder: 0},
		{Type: media.TypeImage, StorageKey: "b", MimeType: "image/png", SizeBytes: 2, SortOrder: 1},
	}
	_, err := newService(tx, &fakePostCreator{}, mediaC).
		CreatePostWithMedia(context.Background(), postInput(), items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tx.committed || tx.rolledBack {
		t.Fatalf("expected commit")
	}
	if len(mediaC.received) != 2 || mediaC.received[0].PostID != "post-1" || mediaC.received[1].PostID != "post-1" {
		t.Fatalf("media items must carry the created post id: %+v", mediaC.received)
	}
}

func TestPostInsertErrorRollsBack(t *testing.T) {
	tx := &fakeTx{}
	_, err := newService(tx, &fakePostCreator{err: errors.New("boom")}, &fakeMediaCreator{}).
		CreatePostWithMedia(context.Background(), postInput(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("expected rollback, got committed=%v rolledBack=%v", tx.committed, tx.rolledBack)
	}
}

func TestMediaInsertErrorRollsBack(t *testing.T) {
	tx := &fakeTx{}
	items := []media.CreateInput{{Type: media.TypeImage, StorageKey: "a", MimeType: "image/png", SizeBytes: 1}}
	_, err := newService(tx, &fakePostCreator{}, &fakeMediaCreator{err: errors.New("boom")}).
		CreatePostWithMedia(context.Background(), postInput(), items)
	if err == nil {
		t.Fatal("expected error")
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("expected rollback")
	}
}

func TestCommitErrorReturnsError(t *testing.T) {
	tx := &fakeTx{commitErr: errors.New("commit failed")}
	_, err := newService(tx, &fakePostCreator{}, &fakeMediaCreator{}).
		CreatePostWithMedia(context.Background(), postInput(), nil)
	if err == nil {
		t.Fatal("expected commit error")
	}
}
