package follow

import (
	"context"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBTX is the minimal executor both *pgxpool.Pool and pgx.Tx satisfy, so a
// follow edge can be created inside a transaction alongside its notification.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// listFollowersQuery lists users who follow $1 (keyset paginated).
const listFollowersQuery = `
SELECT u.id, u.username, u.display_name, u.avatar_url, u.country_code, u.city,
       u.native_language, f.created_at
FROM follows f
JOIN users u ON u.id = f.follower_id
WHERE f.following_id = $1
  AND ($2::timestamptz IS NULL
       OR f.created_at < $2
       OR (f.created_at = $2 AND f.follower_id < $3::uuid))
ORDER BY f.created_at DESC, f.follower_id DESC
LIMIT $4
`

// listFollowingQuery lists users that $1 follows (keyset paginated).
const listFollowingQuery = `
SELECT u.id, u.username, u.display_name, u.avatar_url, u.country_code, u.city,
       u.native_language, f.created_at
FROM follows f
JOIN users u ON u.id = f.following_id
WHERE f.follower_id = $1
  AND ($2::timestamptz IS NULL
       OR f.created_at < $2
       OR (f.created_at = $2 AND f.following_id < $3::uuid))
ORDER BY f.created_at DESC, f.following_id DESC
LIMIT $4
`

// ListFollowers returns up to `limit` users who follow userID.
func (r *PostgresRepository) ListFollowers(ctx context.Context, userID string, cur *Cursor, limit int) ([]ListItem, error) {
	return r.list(ctx, listFollowersQuery, userID, cur, limit)
}

// ListFollowing returns up to `limit` users userID follows.
func (r *PostgresRepository) ListFollowing(ctx context.Context, userID string, cur *Cursor, limit int) ([]ListItem, error) {
	return r.list(ctx, listFollowingQuery, userID, cur, limit)
}

func (r *PostgresRepository) list(ctx context.Context, query, userID string, cur *Cursor, limit int) ([]ListItem, error) {
	var ts, uid any
	if cur != nil {
		ts = cur.CreatedAt
		uid = cur.UserID
	}

	rows, err := r.pool.Query(ctx, query, userID, ts, uid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ListItem, 0, limit)
	for rows.Next() {
		var it ListItem
		if err := rows.Scan(
			&it.ID, &it.Username, &it.DisplayName, &it.AvatarURL,
			&it.CountryCode, &it.City, &it.NativeLanguage, &it.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

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

const followQuery = `
INSERT INTO follows (follower_id, following_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING
`

// Follow inserts a follow edge, ignoring duplicates (idempotent).
func (r *PostgresRepository) Follow(ctx context.Context, followerID, followingID string) error {
	_, err := r.pool.Exec(ctx, followQuery, followerID, followingID)
	return err
}

// FollowTx inserts a follow edge using the given executor (so it can share a
// transaction) and reports whether a new edge was actually created. A duplicate
// follow hits ON CONFLICT DO NOTHING and returns created=false.
func (r *PostgresRepository) FollowTx(ctx context.Context, q DBTX, followerID, followingID string) (bool, error) {
	tag, err := q.Exec(ctx, followQuery, followerID, followingID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

const unfollowQuery = `DELETE FROM follows WHERE follower_id = $1 AND following_id = $2`

// Unfollow removes a follow edge; deleting a missing edge is a no-op.
func (r *PostgresRepository) Unfollow(ctx context.Context, followerID, followingID string) error {
	_, err := r.pool.Exec(ctx, unfollowQuery, followerID, followingID)
	return err
}

const isFollowingQuery = `
SELECT EXISTS (
    SELECT 1 FROM follows WHERE follower_id = $1 AND following_id = $2
)
`

// IsFollowing reports whether the follow edge exists.
func (r *PostgresRepository) IsFollowing(ctx context.Context, followerID, followingID string) (bool, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, isFollowingQuery, followerID, followingID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

const countFollowersQuery = `SELECT count(*) FROM follows WHERE following_id = $1`

// CountFollowers returns how many users follow userID.
func (r *PostgresRepository) CountFollowers(ctx context.Context, userID string) (int64, error) {
	var n int64
	if err := r.pool.QueryRow(ctx, countFollowersQuery, userID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

const countFollowingQuery = `SELECT count(*) FROM follows WHERE follower_id = $1`

// CountFollowing returns how many users userID follows.
func (r *PostgresRepository) CountFollowing(ctx context.Context, userID string) (int64, error) {
	var n int64
	if err := r.pool.QueryRow(ctx, countFollowingQuery, userID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
