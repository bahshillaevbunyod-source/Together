package followrequest

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
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
INSERT INTO follow_requests (requester_id, target_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING
`

// Create inserts a pending request, ignoring duplicates (idempotent), and
// reports whether a new row was actually created.
func (r *PostgresRepository) Create(ctx context.Context, requesterID, targetID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, createQuery, requesterID, targetID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

const existsQuery = `
SELECT EXISTS (
    SELECT 1 FROM follow_requests WHERE requester_id = $1 AND target_id = $2
)
`

// Exists reports whether a pending request exists.
func (r *PostgresRepository) Exists(ctx context.Context, requesterID, targetID string) (bool, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, existsQuery, requesterID, targetID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

const deleteQuery = `DELETE FROM follow_requests WHERE requester_id = $1 AND target_id = $2`

// Delete removes a pending request; deleting a missing request is a no-op. It
// reports whether a row existed.
func (r *PostgresRepository) Delete(ctx context.Context, requesterID, targetID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, deleteQuery, requesterID, targetID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// DeleteTx removes a pending request using the given executor (so it can share
// a transaction) and reports whether a row existed.
func (r *PostgresRepository) DeleteTx(ctx context.Context, q DBTX, requesterID, targetID string) (bool, error) {
	tag, err := q.Exec(ctx, deleteQuery, requesterID, targetID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

const countIncomingQuery = `SELECT count(*) FROM follow_requests WHERE target_id = $1`

// CountIncoming returns how many pending requests target userID.
func (r *PostgresRepository) CountIncoming(ctx context.Context, userID string) (int64, error) {
	var n int64
	if err := r.pool.QueryRow(ctx, countIncomingQuery, userID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// listIncomingQuery lists users with a pending request to $1 (keyset
// paginated), excluding any requester in a block relationship with $1 in either
// direction. Keeping that predicate in SQL preserves stable pages and avoids
// per-row block lookups, mirroring the followers/following list queries.
const listIncomingQuery = `
SELECT u.id, u.username, u.display_name, u.avatar_url, u.country_code, u.city,
       u.native_language, fr.created_at
FROM follow_requests fr
JOIN users u ON u.id = fr.requester_id
WHERE fr.target_id = $1
  AND NOT EXISTS (
    SELECT 1 FROM blocks bl
    WHERE (bl.blocker_id = $1 AND bl.blocked_id = u.id)
       OR (bl.blocker_id = u.id AND bl.blocked_id = $1)
  )
  AND ($2::timestamptz IS NULL
       OR fr.created_at < $2
       OR (fr.created_at = $2 AND fr.requester_id < $3::uuid))
ORDER BY fr.created_at DESC, fr.requester_id DESC
LIMIT $4
`

// ListIncoming returns up to `limit` users with a pending request to userID.
func (r *PostgresRepository) ListIncoming(ctx context.Context, userID string, cur *Cursor, limit int) ([]ListItem, error) {
	var ts, uid any
	if cur != nil {
		ts = cur.CreatedAt
		uid = cur.UserID
	}

	rows, err := r.pool.Query(ctx, listIncomingQuery, userID, ts, uid, limit)
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
