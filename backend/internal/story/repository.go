package story

import "context"

// Repository abstracts story persistence so handlers never touch SQL. Every
// method that returns "active" stories enforces both the 24-hour window and the
// access rule (self-or-follows, no block, never pending requests).
type Repository interface {
	// Create inserts a story and returns the stored row.
	Create(ctx context.Context, in CreateInput) (*Story, error)

	// GetByID loads a raw story row by id with no access/expiry checks. It is for
	// internal ownership checks (e.g. before delete), never for serving content.
	// Returns ErrNotFound when no row exists.
	GetByID(ctx context.Context, id string) (*Story, error)

	// GetActiveVisible returns a single active story that the viewer is allowed
	// to see, with the viewer's viewed state. Returns ErrNotFound when the story
	// does not exist, has expired, or is not visible to the viewer.
	GetActiveVisible(ctx context.Context, viewerID, storyID string) (*Item, error)

	// ListFeed returns active stories the viewer may see — the viewer's own plus
	// those of users the viewer follows (accepted) — excluding blocked pairs and
	// expired stories, ordered (created_at DESC, id DESC), keyset-paginated.
	ListFeed(ctx context.Context, viewerID string, cur *Cursor, limit int) ([]Item, error)

	// ListAuthorActive returns one author's active stories with the viewer's
	// viewed state, but only when the viewer may see them (self-or-follows, no
	// block); otherwise it returns an empty slice. Ordered (created_at DESC,
	// id DESC).
	ListAuthorActive(ctx context.Context, viewerID, authorID string) ([]Item, error)

	// CanView reports whether viewerID may see authorID's stories: viewer is the
	// author OR follows the author (accepted), AND no block exists in either
	// direction. It lets the HTTP layer distinguish "no access" (403) from "no
	// active stories" (empty). Expiry is not part of this relationship check.
	CanView(ctx context.Context, viewerID, authorID string) (bool, error)

	// DeleteOwn deletes a story only when it belongs to authorID, and reports
	// whether a row was deleted. Deleting another user's story affects no rows
	// and returns false, so deletion is never permitted for non-owners.
	DeleteOwn(ctx context.Context, authorID, storyID string) (bool, error)

	// RecordView records that viewerID has seen storyID, idempotently. It reports
	// whether a new view row was created (false on a repeat view).
	RecordView(ctx context.Context, storyID, viewerID string) (bool, error)

	// HasViewed reports whether viewerID has already viewed storyID.
	HasViewed(ctx context.Context, storyID, viewerID string) (bool, error)
}
