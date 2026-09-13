package post

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBTX is the minimal executor both *pgxpool.Pool and pgx.Tx satisfy, so a
// method can run either directly on the pool or inside a transaction.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
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

const insertPostQuery = `
INSERT INTO posts (author_id, content, visibility)
VALUES ($1, $2, $3)
RETURNING id, author_id, content, visibility, created_at, updated_at
`

// Create inserts a new post and returns the stored row.
func (r *PostgresRepository) Create(ctx context.Context, in CreateInput) (*Post, error) {
	return insertPost(ctx, r.pool, in)
}

// CreateTx is Create executed on the given transaction/executor.
func (r *PostgresRepository) CreateTx(ctx context.Context, q DBTX, in CreateInput) (*Post, error) {
	return insertPost(ctx, q, in)
}

func insertPost(ctx context.Context, q DBTX, in CreateInput) (*Post, error) {
	row := q.QueryRow(ctx, insertPostQuery, in.AuthorID, in.Content, in.Visibility)

	var p Post
	if err := row.Scan(&p.ID, &p.AuthorID, &p.Content, &p.Visibility, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

const getPostByIDQuery = `
SELECT id, author_id, content, visibility, created_at, updated_at
FROM posts
WHERE id = $1
`

// GetByID loads a post by id. Returns ErrNotFound when no row exists.
func (r *PostgresRepository) GetByID(ctx context.Context, id string) (*Post, error) {
	row := r.pool.QueryRow(ctx, getPostByIDQuery, id)

	var p Post
	if err := row.Scan(&p.ID, &p.AuthorID, &p.Content, &p.Visibility, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

// Update applies a partial update (content and/or visibility) and returns the
// updated row. author_id / created_at / id are never modified.
func (r *PostgresRepository) Update(ctx context.Context, id string, in PostUpdate) (*Post, error) {
	set := make([]string, 0, 2)
	args := make([]any, 0, 3)
	i := 1
	add := func(col string, val any) {
		set = append(set, fmt.Sprintf("%s = $%d", col, i))
		args = append(args, val)
		i++
	}

	if in.Content != nil {
		add("content", *in.Content)
	}
	if in.Visibility != nil {
		add("visibility", *in.Visibility)
	}
	set = append(set, "updated_at = now()")

	query := fmt.Sprintf(
		"UPDATE posts SET %s WHERE id = $%d "+
			"RETURNING id, author_id, content, visibility, created_at, updated_at",
		strings.Join(set, ", "), i,
	)
	args = append(args, id)

	var p Post
	if err := r.pool.QueryRow(ctx, query, args...).Scan(
		&p.ID, &p.AuthorID, &p.Content, &p.Visibility, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

const deletePostQuery = `DELETE FROM posts WHERE id = $1`

// Delete removes the post. Comments and likes are removed by FK ON DELETE
// CASCADE.
func (r *PostgresRepository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, deletePostQuery, id)
	return err
}

// Feed is a following timeline: own posts plus public/followers posts of
// followed authors. Block and follow logic reuse the shared SQL fragments.
var listFeedQuery = `
SELECT p.id, p.author_id, p.content, p.visibility, p.created_at, p.updated_at,
       u.username, u.display_name, u.avatar_url,
       (SELECT count(*) FROM post_likes pl WHERE pl.post_id = p.id) AS likes_count,
       EXISTS (SELECT 1 FROM post_likes plm WHERE plm.post_id = p.id AND plm.user_id = $1) AS liked_by_me,
       (SELECT count(*) FROM post_comments pc WHERE pc.post_id = p.id) AS comments_count
FROM posts p
JOIN users u ON u.id = p.author_id
WHERE (
        p.author_id = $1
        OR (` + SQLFollowsAuthor + ` AND p.visibility IN ('public', 'followers'))
      )
  AND ` + SQLNotBlocked + `
  AND ($2::timestamptz IS NULL
       OR p.created_at < $2
       OR (p.created_at = $2 AND p.id < $3::uuid))
ORDER BY p.created_at DESC, p.id DESC
LIMIT $4
`

// ListFeed returns the viewer's feed items, keyset-paginated.
func (r *PostgresRepository) ListFeed(ctx context.Context, viewerID string, cur *Cursor, limit int) ([]FeedItem, error) {
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

	items := make([]FeedItem, 0, limit)
	for rows.Next() {
		var it FeedItem
		if err := rows.Scan(
			&it.ID, &it.AuthorID, &it.Content, &it.Visibility, &it.CreatedAt, &it.UpdatedAt,
			&it.AuthorUsername, &it.AuthorDisplayName, &it.AuthorAvatarURL,
			&it.LikesCount, &it.LikedByMe, &it.CommentsCount,
		); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}
