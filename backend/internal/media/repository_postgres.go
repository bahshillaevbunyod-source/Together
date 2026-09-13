package media

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

const columns = `id, post_id, type, storage_key, mime_type, size_bytes,
	width, height, duration_ms, sort_order, created_at`

const insertMediaQuery = `
INSERT INTO post_media
	(post_id, type, storage_key, mime_type, size_bytes, width, height, duration_ms, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING ` + columns

// Create inserts a media row and returns it.
func (r *PostgresRepository) Create(ctx context.Context, in CreateInput) (*Media, error) {
	row := r.pool.QueryRow(ctx, insertMediaQuery,
		in.PostID, in.Type, in.StorageKey, in.MimeType, in.SizeBytes,
		in.Width, in.Height, in.DurationMs, in.SortOrder,
	)
	var m Media
	if err := scanMedia(row, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// CreateMany inserts several media rows in one statement, preserving order.
// An empty list touches the database not at all.
func (r *PostgresRepository) CreateMany(ctx context.Context, items []CreateInput) error {
	return createMany(ctx, r.pool, items)
}

// CreateManyTx is CreateMany executed on the given transaction/executor.
func (r *PostgresRepository) CreateManyTx(ctx context.Context, q DBTX, items []CreateInput) error {
	return createMany(ctx, q, items)
}

func createMany(ctx context.Context, q DBTX, items []CreateInput) error {
	if len(items) == 0 {
		return nil
	}
	query, args := buildInsertMany(items)
	if _, err := q.Exec(ctx, query, args...); err != nil {
		return mapInsertError(err)
	}
	return nil
}

// mapInsertError translates a unique-violation on the storage_key constraint
// into ErrMediaAlreadyAttached; everything else is returned unchanged.
func mapInsertError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) &&
		pgErr.Code == "23505" &&
		pgErr.ConstraintName == "post_media_storage_key_unique" {
		return ErrMediaAlreadyAttached
	}
	return err
}

// buildInsertMany builds a multi-row INSERT and its flattened args. Each item
// contributes 9 columns, including sort_order (preserved in list order).
func buildInsertMany(items []CreateInput) (string, []any) {
	var sb strings.Builder
	sb.WriteString("INSERT INTO post_media " +
		"(post_id, type, storage_key, mime_type, size_bytes, width, height, duration_ms, sort_order) VALUES ")

	args := make([]any, 0, len(items)*9)
	p := 1
	for i, it := range items {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			p, p+1, p+2, p+3, p+4, p+5, p+6, p+7, p+8)
		p += 9
		args = append(args,
			it.PostID, it.Type, it.StorageKey, it.MimeType, it.SizeBytes,
			it.Width, it.Height, it.DurationMs, it.SortOrder,
		)
	}
	return sb.String(), args
}

const listByPostQuery = `
SELECT ` + columns + `
FROM post_media
WHERE post_id = $1
ORDER BY sort_order ASC, id ASC
`

// ListByPost returns a post's media ordered by sort_order.
func (r *PostgresRepository) ListByPost(ctx context.Context, postID string) ([]Media, error) {
	rows, err := r.pool.Query(ctx, listByPostQuery, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectMedia(rows)
}

const listByPostIDsQuery = `
SELECT ` + columns + `
FROM post_media
WHERE post_id = ANY($1)
ORDER BY post_id, sort_order ASC, id ASC
`

// ListByPostIDs returns media for many posts in a single query.
func (r *PostgresRepository) ListByPostIDs(ctx context.Context, postIDs []string) ([]Media, error) {
	if len(postIDs) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, listByPostIDsQuery, postIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectMedia(rows)
}

type scannable interface {
	Scan(dest ...any) error
}

func scanMedia(row scannable, m *Media) error {
	return row.Scan(
		&m.ID, &m.PostID, &m.Type, &m.StorageKey, &m.MimeType, &m.SizeBytes,
		&m.Width, &m.Height, &m.DurationMs, &m.SortOrder, &m.CreatedAt,
	)
}

func collectMedia(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]Media, error) {
	var items []Media
	for rows.Next() {
		var m Media
		if err := scanMedia(rows, &m); err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}
