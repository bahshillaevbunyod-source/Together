package like

import (
	"context"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBTX is the minimal executor both *pgxpool.Pool and pgx.Tx satisfy, so a like
// can be created inside a transaction alongside its notification.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
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

const likeQuery = `
INSERT INTO post_likes (post_id, user_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING
`

// LikePost inserts a like, ignoring duplicates (idempotent).
func (r *PostgresRepository) LikePost(ctx context.Context, userID, postID string) error {
	_, err := r.pool.Exec(ctx, likeQuery, postID, userID)
	return err
}

// LikeTx inserts a like using the given executor (so it can share a
// transaction) and reports whether a new like was actually created. A duplicate
// like hits ON CONFLICT DO NOTHING and returns created=false.
func (r *PostgresRepository) LikeTx(ctx context.Context, q DBTX, userID, postID string) (bool, error) {
	tag, err := q.Exec(ctx, likeQuery, postID, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

const unlikeQuery = `DELETE FROM post_likes WHERE post_id = $1 AND user_id = $2`

// UnlikePost removes a like; deleting a missing like is a no-op.
func (r *PostgresRepository) UnlikePost(ctx context.Context, userID, postID string) error {
	_, err := r.pool.Exec(ctx, unlikeQuery, postID, userID)
	return err
}

const isPostLikedQuery = `
SELECT EXISTS (
    SELECT 1 FROM post_likes WHERE post_id = $1 AND user_id = $2
)
`

// IsPostLiked reports whether userID likes postID.
func (r *PostgresRepository) IsPostLiked(ctx context.Context, userID, postID string) (bool, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, isPostLikedQuery, postID, userID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

const countPostLikesQuery = `SELECT count(*) FROM post_likes WHERE post_id = $1`

// CountPostLikes returns the number of likes on postID.
func (r *PostgresRepository) CountPostLikes(ctx context.Context, postID string) (int64, error) {
	var n int64
	if err := r.pool.QueryRow(ctx, countPostLikesQuery, postID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
