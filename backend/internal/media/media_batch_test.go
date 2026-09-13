package media

import (
	"context"
	"strings"
	"testing"
)

func TestCreateManyEmpty(t *testing.T) {
	// Empty list must be a no-op: no SQL, so a nil pool is never touched.
	repo := &PostgresRepository{}
	if err := repo.CreateMany(context.Background(), nil); err != nil {
		t.Fatalf("empty CreateMany should be nil, got %v", err)
	}
	if err := repo.CreateMany(context.Background(), []CreateInput{}); err != nil {
		t.Fatalf("empty CreateMany should be nil, got %v", err)
	}
}

func TestBuildInsertManyOne(t *testing.T) {
	items := []CreateInput{
		{PostID: "p1", Type: TypeImage, StorageKey: "k", MimeType: "image/png", SizeBytes: 10, SortOrder: 0},
	}
	query, args := buildInsertMany(items)

	if len(args) != 9 {
		t.Fatalf("expected 9 args, got %d", len(args))
	}
	if !strings.Contains(query, "($1,$2,$3,$4,$5,$6,$7,$8,$9)") {
		t.Fatalf("unexpected query: %s", query)
	}
	if args[8] != 0 { // sort_order
		t.Fatalf("sort_order arg wrong: %v", args[8])
	}
}

func TestBuildInsertManyMultiple(t *testing.T) {
	items := []CreateInput{
		{PostID: "p1", Type: TypeImage, StorageKey: "a", MimeType: "image/png", SizeBytes: 1, SortOrder: 0},
		{PostID: "p1", Type: TypeImage, StorageKey: "b", MimeType: "image/png", SizeBytes: 2, SortOrder: 1},
		{PostID: "p1", Type: TypeVideo, StorageKey: "c", MimeType: "video/mp4", SizeBytes: 3, SortOrder: 2},
	}
	query, args := buildInsertMany(items)

	if len(args) != 27 {
		t.Fatalf("expected 27 args, got %d", len(args))
	}
	// three value tuples -> the third starts at $19
	if !strings.Contains(query, "$19") || strings.Contains(query, "$28") {
		t.Fatalf("unexpected placeholder set: %s", query)
	}
	// sort_order preserved per item (index 8, 17, 26)
	if args[8] != 0 || args[17] != 1 || args[26] != 2 {
		t.Fatalf("sort_order not preserved: %v %v %v", args[8], args[17], args[26])
	}
	// storage keys preserved in order (index 2, 11, 20)
	if args[2] != "a" || args[11] != "b" || args[20] != "c" {
		t.Fatalf("order not preserved: %v %v %v", args[2], args[11], args[20])
	}
}
