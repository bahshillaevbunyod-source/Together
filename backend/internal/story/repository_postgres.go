package story

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Shared SQL fragments. Keeping the access rule and active window in one place
// guarantees every read path enforces identical semantics, and lets tests assert
// the exact predicates.
const (
	// sqlActiveWindow restricts to stories created within the last 24 hours.
	// `s` is the stories alias. This is the sole expiry mechanism.
	sqlActiveWindow = `s.created_at > now() - interval '24 hours'`

	// sqlViewerCanSeeAuthor is true when the viewer ($1) is the author or has an
	// accepted follows edge to the author, AND no block exists in either
	// direction. It deliberately never references follow_requests, so a pending
	// request grants no access.
	sqlViewerCanSeeAuthor = `(
        s.author_id = $1
        OR s.author_id IN (SELECT following_id FROM follows WHERE follower_id = $1)
    )
    AND NOT EXISTS (
        SELECT 1 FROM blocks bl
        WHERE (bl.blocker_id = $1 AND bl.blocked_id = s.author_id)
           OR (bl.blocker_id = s.author_id AND bl.blocked_id = $1)
    )`

	// sqlItemColumns is the projection for an Item: story row + author public
	// fields + the viewer's ($1) viewed state.
	sqlItemColumns = `s.id, s.author_id, s.type, s.storage_key, s.mime_type,
       s.width, s.height, s.duration_ms, s.created_at,
       u.username, u.display_name, u.avatar_url,
       EXISTS (
           SELECT 1 FROM story_views sv
           WHERE sv.story_id = s.id AND sv.viewer_id = $1
       ) AS viewed`
)

// PostgresRepository is the production Repository backed by pgxpool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// Compile-time assurance that the production repository satisfies Repository.
var _ Repository = (*PostgresRepository)(nil)

// NewPostgresRepository builds a Repository over the given connection pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const createQuery = `
INSERT INTO stories (author_id, type, storage_key, mime_type, width, height, duration_ms)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, author_id, type, storage_key, mime_type, width, height, duration_ms, created_at
`

// Create inserts a story and returns the stored row.
func (r *PostgresRepository) Create(ctx context.Context, in CreateInput) (*Story, error) {
	row := r.pool.QueryRow(ctx, createQuery,
		in.AuthorID, in.Type, in.StorageKey, in.MimeType, in.Width, in.Height, in.DurationMs,
	)
	var s Story
	if err := row.Scan(
		&s.ID, &s.AuthorID, &s.Type, &s.StorageKey, &s.MimeType,
		&s.Width, &s.Height, &s.DurationMs, &s.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &s, nil
}

const getByIDQuery = `
SELECT id, author_id, type, storage_key, mime_type, width, height, duration_ms, created_at
FROM stories
WHERE id = $1
`

// GetByID loads a raw story row (no access/expiry checks). Returns ErrNotFound
// when no row exists.
func (r *PostgresRepository) GetByID(ctx context.Context, id string) (*Story, error) {
	var s Story
	if err := r.pool.QueryRow(ctx, getByIDQuery, id).Scan(
		&s.ID, &s.AuthorID, &s.Type, &s.StorageKey, &s.MimeType,
		&s.Width, &s.Height, &s.DurationMs, &s.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}

// getActiveVisibleQuery: $1 = viewer, $2 = story id.
const getActiveVisibleQuery = `
SELECT ` + sqlItemColumns + `
FROM stories s
JOIN users u ON u.id = s.author_id
WHERE s.id = $2
  AND ` + sqlActiveWindow + `
  AND ` + sqlViewerCanSeeAuthor + `
`

// GetActiveVisible returns a single active, visible story with viewed state.
func (r *PostgresRepository) GetActiveVisible(ctx context.Context, viewerID, storyID string) (*Item, error) {
	var it Item
	if err := scanItem(r.pool.QueryRow(ctx, getActiveVisibleQuery, viewerID, storyID), &it); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &it, nil
}

// listFeedQuery: $1 = viewer, $2 = cursor created_at (nullable), $3 = cursor id
// (nullable), $4 = limit. Own + followed authors' active stories, block-filtered,
// keyset-paginated, newest first.
const listFeedQuery = `
SELECT ` + sqlItemColumns + `
FROM stories s
JOIN users u ON u.id = s.author_id
WHERE ` + sqlActiveWindow + `
  AND ` + sqlViewerCanSeeAuthor + `
  AND ($2::timestamptz IS NULL
       OR s.created_at < $2
       OR (s.created_at = $2 AND s.id < $3::uuid))
ORDER BY s.created_at DESC, s.id DESC
LIMIT $4
`

// ListFeed returns active, visible stories keyset-paginated newest-first.
func (r *PostgresRepository) ListFeed(ctx context.Context, viewerID string, cur *Cursor, limit int) ([]Item, error) {
	var ts, id any
	if cur != nil {
		ts = cur.CreatedAt
		id = cur.ID
	}
	rows, err := r.pool.Query(ctx, listFeedQuery, viewerID, ts, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanItems(rows, limit)
}

// listAuthorActiveQuery: $1 = viewer, $2 = author id. The access predicate uses
// $1 (the viewer) exactly as the other paths, so it also covers the self case
// and the block check; a viewer without access simply gets no rows.
const listAuthorActiveQuery = `
SELECT ` + sqlItemColumns + `
FROM stories s
JOIN users u ON u.id = s.author_id
WHERE s.author_id = $2
  AND ` + sqlActiveWindow + `
  AND ` + sqlViewerCanSeeAuthor + `
ORDER BY s.created_at DESC, s.id DESC
`

// ListAuthorActive returns one author's active stories when visible to viewer.
func (r *PostgresRepository) ListAuthorActive(ctx context.Context, viewerID, authorID string) ([]Item, error) {
	rows, err := r.pool.Query(ctx, listAuthorActiveQuery, viewerID, authorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanItems(rows, 0)
}

// canViewQuery mirrors sqlViewerCanSeeAuthor as a standalone relationship check
// ($1 = viewer, $2 = author) with no story row in scope, so it names the author
// column directly. Expiry is intentionally not part of this check.
const canViewQuery = `
SELECT (
    ($1 = $2 OR EXISTS (SELECT 1 FROM follows WHERE follower_id = $1 AND following_id = $2))
    AND NOT EXISTS (
        SELECT 1 FROM blocks bl
        WHERE (bl.blocker_id = $1 AND bl.blocked_id = $2)
           OR (bl.blocker_id = $2 AND bl.blocked_id = $1)
    )
)
`

// CanView reports whether viewerID may see authorID's stories.
func (r *PostgresRepository) CanView(ctx context.Context, viewerID, authorID string) (bool, error) {
	var ok bool
	if err := r.pool.QueryRow(ctx, canViewQuery, viewerID, authorID).Scan(&ok); err != nil {
		return false, err
	}
	return ok, nil
}

const deleteOwnQuery = `DELETE FROM stories WHERE id = $1 AND author_id = $2`

// DeleteOwn deletes a story only when it belongs to authorID; reports whether a
// row was deleted.
func (r *PostgresRepository) DeleteOwn(ctx context.Context, authorID, storyID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, deleteOwnQuery, storyID, authorID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

const recordViewQuery = `
INSERT INTO story_views (story_id, viewer_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING
`

// RecordView records a view idempotently; reports whether a new row was created.
func (r *PostgresRepository) RecordView(ctx context.Context, storyID, viewerID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, recordViewQuery, storyID, viewerID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

const hasViewedQuery = `
SELECT EXISTS (
    SELECT 1 FROM story_views WHERE story_id = $1 AND viewer_id = $2
)
`

// HasViewed reports whether viewerID has already viewed storyID.
func (r *PostgresRepository) HasViewed(ctx context.Context, storyID, viewerID string) (bool, error) {
	var seen bool
	if err := r.pool.QueryRow(ctx, hasViewedQuery, storyID, viewerID).Scan(&seen); err != nil {
		return false, err
	}
	return seen, nil
}

// scanItem scans one Item row in the sqlItemColumns order.
func scanItem(row pgx.Row, it *Item) error {
	return row.Scan(
		&it.ID, &it.AuthorID, &it.Type, &it.StorageKey, &it.MimeType,
		&it.Width, &it.Height, &it.DurationMs, &it.CreatedAt,
		&it.AuthorUsername, &it.AuthorDisplayName, &it.AuthorAvatarURL, &it.Viewed,
	)
}

// scanItems collects Item rows. capHint pre-sizes the slice when known (>0).
func scanItems(rows pgx.Rows, capHint int) ([]Item, error) {
	items := make([]Item, 0, capHint)
	for rows.Next() {
		var it Item
		if err := scanItem(rows, &it); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}
