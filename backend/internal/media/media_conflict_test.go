package media

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapInsertErrorUniqueStorageKey(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "23505", ConstraintName: "post_media_storage_key_unique"}
	if got := mapInsertError(pgErr); !errors.Is(got, ErrMediaAlreadyAttached) {
		t.Fatalf("expected ErrMediaAlreadyAttached, got %v", got)
	}
}

func TestMapInsertErrorOtherUnique(t *testing.T) {
	// A different unique constraint must not be mapped.
	pgErr := &pgconn.PgError{Code: "23505", ConstraintName: "some_other_key"}
	if got := mapInsertError(pgErr); errors.Is(got, ErrMediaAlreadyAttached) {
		t.Fatalf("must not map other constraint to ErrMediaAlreadyAttached")
	}
}

func TestMapInsertErrorPassthrough(t *testing.T) {
	boom := errors.New("boom")
	if got := mapInsertError(boom); got != boom {
		t.Fatalf("expected passthrough, got %v", got)
	}
}
