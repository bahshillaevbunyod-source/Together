package comment

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBTX is the minimal executor both *pgxpool.Pool and pgx.Tx satisfy, so a
// comment can be created inside a transaction alongside its notification.
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

const insertCommentQuery = `
INSERT INTO post_comments (post_id, author_id, content)
VALUES ($1, $2, $3)
RETURNING id, post_id, author_id, content, created_at, updated_at
`

// Create inserts a new comment and returns the stored row.
func (r *PostgresRepository) Create(ctx context.Context, in CreateInput) (*Comment, error) {
	return r.createWith(ctx, r.pool, in)
}

// CreateTx inserts a new comment using the given executor so it can share a
// transaction with the notification it triggers.
func (r *PostgresRepository) CreateTx(ctx context.Context, q DBTX, in CreateInput) (*Comment, error) {
	return r.createWith(ctx, q, in)
}

func (r *PostgresRepository) createWith(ctx context.Context, q DBTX, in CreateInput) (*Comment, error) {
	row := q.QueryRow(ctx, insertCommentQuery, in.PostID, in.AuthorID, in.Content)

	var c Comment
	if err := row.Scan(&c.ID, &c.PostID, &c.AuthorID, &c.Content, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

const listCommentsQuery = `
SELECT c.id, c.author_id, c.content, c.created_at, c.updated_at,
       u.username, u.display_name, u.avatar_url
FROM post_comments c
JOIN users u ON u.id = c.author_id
WHERE c.post_id = $1
  AND ($2::timestamptz IS NULL
       OR c.created_at < $2
       OR (c.created_at = $2 AND c.id < $3::uuid))
ORDER BY c.created_at DESC, c.id DESC
LIMIT $4
`

// ListByPost returns up to `limit` comments for a post, keyset-paginated.
func (r *PostgresRepository) ListByPost(ctx context.Context, postID string, cur *Cursor, limit int) ([]ListItem, error) {
	var ts, id any
	if cur != nil {
		ts = cur.CreatedAt
		id = cur.ID
	}

	rows, err := r.pool.Query(ctx, listCommentsQuery, postID, ts, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ListItem, 0, limit)
	for rows.Next() {
		var it ListItem
		if err := rows.Scan(
			&it.ID, &it.AuthorID, &it.Content, &it.CreatedAt, &it.UpdatedAt,
			&it.AuthorUsername, &it.AuthorDisplayName, &it.AuthorAvatarURL,
		); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

const getCommentByIDQuery = `
SELECT id, post_id, author_id, content, created_at, updated_at
FROM post_comments
WHERE id = $1
`

// GetByID loads a comment by id. Returns ErrNotFound when no row exists.
func (r *PostgresRepository) GetByID(ctx context.Context, id string) (*Comment, error) {
	row := r.pool.QueryRow(ctx, getCommentByIDQuery, id)

	var c Comment
	if err := row.Scan(&c.ID, &c.PostID, &c.AuthorID, &c.Content, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

// updateCommentQuery updates only content + updated_at; author_id, post_id and
// created_at are never modified.
const updateCommentQuery = `
UPDATE post_comments
SET content = $2, updated_at = now()
WHERE id = $1
RETURNING id, post_id, author_id, content, created_at, updated_at
`

// UpdateContent updates the comment's content and returns the updated row.
func (r *PostgresRepository) UpdateContent(ctx context.Context, id, content string) (*Comment, error) {
	row := r.pool.QueryRow(ctx, updateCommentQuery, id, content)

	var c Comment
	if err := row.Scan(&c.ID, &c.PostID, &c.AuthorID, &c.Content, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

const deleteCommentQuery = `DELETE FROM post_comments WHERE id = $1`

// Delete removes the comment.
func (r *PostgresRepository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, deleteCommentQuery, id)
	return err
}

const countCommentsQuery = `SELECT count(*) FROM post_comments WHERE post_id = $1`

// CountByPost returns the number of comments on a post.
func (r *PostgresRepository) CountByPost(ctx context.Context, postID string) (int64, error) {
	var n int64
	if err := r.pool.QueryRow(ctx, countCommentsQuery, postID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
