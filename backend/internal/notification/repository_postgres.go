package notification

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBTX is the minimal executor both *pgxpool.Pool and pgx.Tx satisfy, so a
// notification can be created inside an event transaction later.
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

const insertQuery = `
INSERT INTO notifications (user_id, actor_id, type, post_id, comment_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, user_id, actor_id, type, post_id, comment_id, read_at, created_at
`

// Create inserts a notification and returns the stored row.
func (r *PostgresRepository) Create(ctx context.Context, in CreateInput) (*Notification, error) {
	row := r.db.QueryRow(ctx, insertQuery, in.UserID, in.ActorID, in.Type, in.PostID, in.CommentID)

	var n Notification
	if err := row.Scan(
		&n.ID, &n.UserID, &n.ActorID, &n.Type, &n.PostID, &n.CommentID, &n.ReadAt, &n.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &n, nil
}

const insertTxQuery = `
INSERT INTO notifications (user_id, actor_id, type, post_id, comment_id)
VALUES ($1, $2, $3, $4, $5)
`

// CreateTx inserts a notification using the given executor so it can share a
// transaction with the action that triggered it. It does not return the row.
func (r *PostgresRepository) CreateTx(ctx context.Context, q DBTX, in CreateInput) error {
	_, err := q.Exec(ctx, insertTxQuery, in.UserID, in.ActorID, in.Type, in.PostID, in.CommentID)
	return err
}

const listQuery = `
SELECT n.id, n.type, n.post_id, n.comment_id, n.read_at, n.created_at,
       n.actor_id, u.username, u.display_name, u.avatar_url
FROM notifications n
LEFT JOIN users u ON u.id = n.actor_id
WHERE n.user_id = $1
  AND ($2::timestamptz IS NULL
       OR n.created_at < $2
       OR (n.created_at = $2 AND n.id < $3::uuid))
ORDER BY n.created_at DESC, n.id DESC
LIMIT $4
`

// List returns a user's notifications newest-first, keyset-paginated.
func (r *PostgresRepository) List(ctx context.Context, userID string, cur *Cursor, limit int) ([]ListItem, error) {
	var ts, id any
	if cur != nil {
		ts = cur.CreatedAt
		id = cur.ID
	}

	rows, err := r.db.Query(ctx, listQuery, userID, ts, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ListItem, 0, limit)
	for rows.Next() {
		var it ListItem
		if err := rows.Scan(
			&it.ID, &it.Type, &it.PostID, &it.CommentID, &it.ReadAt, &it.CreatedAt,
			&it.ActorID, &it.ActorUsername, &it.ActorDisplayName, &it.ActorAvatarURL,
		); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

const markReadQuery = `
UPDATE notifications
SET read_at = COALESCE(read_at, now())
WHERE id = $1 AND user_id = $2
`

// MarkRead marks a user's own notification read, preserving the first read_at
// via COALESCE (so repeats are idempotent). Returns whether a row matched.
func (r *PostgresRepository) MarkRead(ctx context.Context, userID, notificationID string) (bool, error) {
	tag, err := r.db.Exec(ctx, markReadQuery, notificationID, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

const markAllReadQuery = `
UPDATE notifications
SET read_at = now()
WHERE user_id = $1 AND read_at IS NULL
`

// MarkAllRead marks all of a user's unread notifications read. Idempotent:
// already-read rows are excluded by the WHERE clause.
func (r *PostgresRepository) MarkAllRead(ctx context.Context, userID string) error {
	_, err := r.db.Exec(ctx, markAllReadQuery, userID)
	return err
}

const countUnreadQuery = `SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`

// CountUnread returns the number of unread notifications for a user.
func (r *PostgresRepository) CountUnread(ctx context.Context, userID string) (int64, error) {
	var n int64
	if err := r.db.QueryRow(ctx, countUnreadQuery, userID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
