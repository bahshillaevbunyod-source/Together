package bookmark

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"together/backend/internal/post"
)

// DBTX is the minimal executor both *pgxpool.Pool and pgx.Tx satisfy.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// PostgresRepository is the production Repository backed by pgxpool.
type PostgresRepository struct {
	db DBTX
}

// Compile-time assurance that the production repository satisfies Repository.
var _ Repository = (*PostgresRepository)(nil)

// NewPostgresRepository builds a Repository over the given connection pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: pool}
}

const saveQuery = `
INSERT INTO post_bookmarks (user_id, post_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING
`

// Save bookmarks a post, ignoring duplicates (idempotent).
func (r *PostgresRepository) Save(ctx context.Context, userID, postID string) error {
	_, err := r.db.Exec(ctx, saveQuery, userID, postID)
	return err
}

const removeQuery = `DELETE FROM post_bookmarks WHERE user_id = $1 AND post_id = $2`

// Remove deletes a bookmark; deleting a missing one is a no-op.
func (r *PostgresRepository) Remove(ctx context.Context, userID, postID string) error {
	_, err := r.db.Exec(ctx, removeQuery, userID, postID)
	return err
}

const isSavedQuery = `
SELECT EXISTS (
    SELECT 1 FROM post_bookmarks WHERE user_id = $1 AND post_id = $2
)
`

// IsSaved reports whether the user has bookmarked the post.
func (r *PostgresRepository) IsSaved(ctx context.Context, userID, postID string) (bool, error) {
	var exists bool
	if err := r.db.QueryRow(ctx, isSavedQuery, userID, postID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// listSavedItemsQuery returns the user's bookmarked, accessible posts with
// display counts, keyset-paginated by (bookmark created_at DESC, post id DESC).
// Bookmarks list shows any accessible bookmarked post (own OR public OR
// followers-of-followed). Block and follow logic reuse the shared SQL fragments.
var listSavedItemsQuery = `
SELECT p.id, p.author_id, p.content, p.visibility, p.created_at, p.updated_at,
       u.username, u.display_name, u.avatar_url,
       (SELECT count(*) FROM post_likes pl WHERE pl.post_id = p.id) AS likes_count,
       EXISTS (SELECT 1 FROM post_likes plm WHERE plm.post_id = p.id AND plm.user_id = $1) AS liked_by_me,
       (SELECT count(*) FROM post_comments pc WHERE pc.post_id = p.id) AS comments_count,
       b.created_at
FROM post_bookmarks b
JOIN posts p ON p.id = b.post_id
JOIN users u ON u.id = p.author_id
WHERE b.user_id = $1
  AND (
        p.author_id = $1
        OR p.visibility = 'public'
        OR (p.visibility = 'followers' AND ` + post.SQLFollowsAuthor + `)
      )
  AND ` + post.SQLNotBlocked + `
  AND ($2::timestamptz IS NULL
       OR b.created_at < $2
       OR (b.created_at = $2 AND p.id < $3::uuid))
ORDER BY b.created_at DESC, p.id DESC
LIMIT $4
`

// ListSaved returns the user's bookmarked, accessible posts, keyset-paginated.
func (r *PostgresRepository) ListSaved(ctx context.Context, userID string, cur *post.Cursor, limit int) ([]SavedItem, error) {
	var ts, id any
	if cur != nil {
		ts = cur.CreatedAt
		id = cur.ID
	}

	rows, err := r.db.Query(ctx, listSavedItemsQuery, userID, ts, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]SavedItem, 0, limit)
	for rows.Next() {
		var it SavedItem
		if err := rows.Scan(
			&it.ID, &it.AuthorID, &it.Content, &it.Visibility, &it.CreatedAt, &it.UpdatedAt,
			&it.AuthorUsername, &it.AuthorDisplayName, &it.AuthorAvatarURL,
			&it.LikesCount, &it.LikedByMe, &it.CommentsCount, &it.BookmarkedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

const listSavedQuery = `SELECT post_id FROM post_bookmarks WHERE user_id = $1 AND post_id = ANY($2)`

// ListSavedPostIDs returns which of postIDs the user has bookmarked, in one
// query. An empty postIDs list returns an empty map without touching the DB.
func (r *PostgresRepository) ListSavedPostIDs(ctx context.Context, userID string, postIDs []string) (map[string]bool, error) {
	saved := make(map[string]bool, len(postIDs))
	if len(postIDs) == 0 {
		return saved, nil
	}
	rows, err := r.db.Query(ctx, listSavedQuery, userID, postIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		saved[id] = true
	}
	return saved, rows.Err()
}
