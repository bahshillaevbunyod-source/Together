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
       ) AS viewed,
       EXISTS (
           SELECT 1 FROM story_likes sl
           WHERE sl.story_id = s.id AND sl.user_id = $1
       ) AS liked_by_me,
       CASE WHEN s.author_id = $1 THEN (
           SELECT count(*) FROM story_views vc
           WHERE vc.story_id = s.id
             AND vc.viewer_id <> s.author_id
             AND NOT EXISTS (
                 SELECT 1 FROM blocks vb
                 WHERE (vb.blocker_id = s.author_id AND vb.blocked_id = vc.viewer_id)
                    OR (vb.blocker_id = vc.viewer_id AND vb.blocked_id = s.author_id)
             )
       ) ELSE 0 END AS view_count`
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

// recordViewQuery inserts only for an ACTIVE story the viewer did not author,
// so the owner never counts as a viewer and an expired story takes no new
// views even if the caller raced the expiry. Idempotent via the primary key.
const recordViewQuery = `
INSERT INTO story_views (story_id, viewer_id)
SELECT s.id, $2
FROM stories s
WHERE s.id = $1
  AND s.author_id <> $2
  AND ` + sqlActiveWindow + `
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

// likeQuery mirrors recordViewQuery: active story, not the author, idempotent.
const likeQuery = `
INSERT INTO story_likes (story_id, user_id)
SELECT s.id, $2
FROM stories s
WHERE s.id = $1
  AND s.author_id <> $2
  AND ` + sqlActiveWindow + `
ON CONFLICT DO NOTHING
`

// Like records a like idempotently; reports whether a new row was created.
func (r *PostgresRepository) Like(ctx context.Context, storyID, userID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, likeQuery, storyID, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

const unlikeQuery = `DELETE FROM story_likes WHERE story_id = $1 AND user_id = $2`

// Unlike removes a like; reports whether a row was removed.
func (r *PostgresRepository) Unlike(ctx context.Context, storyID, userID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, unlikeQuery, storyID, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// sqlViewerFrom + sqlViewerWhere define the owner-visible viewer set of story $1: other people's
// views only, excluding any blocked pair (either direction). Aliases: sv =
// story_views, s = stories.
const sqlViewerFrom = `
FROM story_views sv
JOIN stories s ON s.id = sv.story_id
JOIN users u ON u.id = sv.viewer_id`

const sqlViewerWhere = `
WHERE sv.story_id = $1
  AND sv.viewer_id <> s.author_id
  AND NOT EXISTS (
      SELECT 1 FROM blocks bl
      WHERE (bl.blocker_id = s.author_id AND bl.blocked_id = sv.viewer_id)
         OR (bl.blocker_id = sv.viewer_id AND bl.blocked_id = s.author_id)
  )`

// listViewersQuery: $1 = story, $2/$3 = cursor (nullable), $4 = limit.
const listViewersQuery = `
SELECT u.id, u.username, u.display_name, u.avatar_url, sv.viewed_at,
       EXISTS (
           SELECT 1 FROM story_likes sl
           WHERE sl.story_id = sv.story_id AND sl.user_id = sv.viewer_id
       ) AS liked
` + sqlViewerFrom + sqlViewerWhere + `
  AND ($2::timestamptz IS NULL
       OR sv.viewed_at < $2
       OR (sv.viewed_at = $2 AND sv.viewer_id < $3::uuid))
ORDER BY sv.viewed_at DESC, sv.viewer_id DESC
LIMIT $4
`

// ListViewers returns the owner-visible viewers newest-first.
func (r *PostgresRepository) ListViewers(ctx context.Context, storyID string, cur *ViewerCursor, limit int) ([]Viewer, error) {
	var ts, id any
	if cur != nil {
		ts = cur.ViewedAt
		id = cur.UserID
	}
	rows, err := r.pool.Query(ctx, listViewersQuery, storyID, ts, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Viewer, 0, limit)
	for rows.Next() {
		var v Viewer
		if err := rows.Scan(&v.UserID, &v.Username, &v.DisplayName, &v.AvatarURL, &v.ViewedAt, &v.Liked); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

const countViewersQuery = `SELECT count(*)` + sqlViewerFrom + sqlViewerWhere

// CountViewers counts the owner-visible viewer set.
func (r *PostgresRepository) CountViewers(ctx context.Context, storyID string) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx, countViewersQuery, storyID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

const recordReplyQuery = `
INSERT INTO story_replies (message_id, story_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING
`

// RecordReply links a message to the story it replied to.
func (r *PostgresRepository) RecordReply(ctx context.Context, messageID, storyID string) error {
	_, err := r.pool.Exec(ctx, recordReplyQuery, messageID, storyID)
	return err
}

const replyRefsQuery = `
SELECT message_id, story_id FROM story_replies WHERE message_id = ANY($1::uuid[])
`

// ReplyRefs returns story references for the given message ids.
func (r *PostgresRepository) ReplyRefs(ctx context.Context, messageIDs []string) (map[string]*string, error) {
	out := map[string]*string{}
	if len(messageIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, replyRefsQuery, messageIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var mid string
		var sid *string
		if err := rows.Scan(&mid, &sid); err != nil {
			return nil, err
		}
		out[mid] = sid
	}
	return out, rows.Err()
}

// scanItem scans one Item row in the sqlItemColumns order.
func scanItem(row pgx.Row, it *Item) error {
	return row.Scan(
		&it.ID, &it.AuthorID, &it.Type, &it.StorageKey, &it.MimeType,
		&it.Width, &it.Height, &it.DurationMs, &it.CreatedAt,
		&it.AuthorUsername, &it.AuthorDisplayName, &it.AuthorAvatarURL, &it.Viewed,
		&it.LikedByMe, &it.ViewCount,
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
