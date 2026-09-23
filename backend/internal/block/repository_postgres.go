package block

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

const insertBlockQuery = `
INSERT INTO blocks (blocker_id, blocked_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING
`

// deleteFollowsBothWays removes follow edges in either direction between the
// two users.
const deleteFollowsBothWays = `
DELETE FROM follows
WHERE (follower_id = $1 AND following_id = $2)
   OR (follower_id = $2 AND following_id = $1)
`

// deleteFollowRequestsBothWays removes any pending follow request in either
// direction between the two users. A block must clear pending requests as well
// as accepted edges, so a blocked pair has no lingering way to gain access.
const deleteFollowRequestsBothWays = `
DELETE FROM follow_requests
WHERE (requester_id = $1 AND target_id = $2)
   OR (requester_id = $2 AND target_id = $1)
`

// Block inserts the block edge and removes both follow edges and pending follow
// requests in both directions, atomically in a single transaction. Idempotent.
func (r *PostgresRepository) Block(ctx context.Context, blockerID, blockedID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // no-op after a successful Commit

	if _, err := tx.Exec(ctx, insertBlockQuery, blockerID, blockedID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, deleteFollowsBothWays, blockerID, blockedID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, deleteFollowRequestsBothWays, blockerID, blockedID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const unblockQuery = `DELETE FROM blocks WHERE blocker_id = $1 AND blocked_id = $2`

// Unblock removes a block edge; deleting a missing edge is a no-op.
func (r *PostgresRepository) Unblock(ctx context.Context, blockerID, blockedID string) error {
	_, err := r.pool.Exec(ctx, unblockQuery, blockerID, blockedID)
	return err
}

const isBlockedQuery = `
SELECT EXISTS (
    SELECT 1 FROM blocks WHERE blocker_id = $1 AND blocked_id = $2
)
`

// IsBlocked reports whether blockerID blocks blockedID.
func (r *PostgresRepository) IsBlocked(ctx context.Context, blockerID, blockedID string) (bool, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, isBlockedQuery, blockerID, blockedID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

const hasBlockBetweenQuery = `
SELECT EXISTS (
    SELECT 1 FROM blocks
    WHERE (blocker_id = $1 AND blocked_id = $2)
       OR (blocker_id = $2 AND blocked_id = $1)
)
`

// HasBlockBetween reports whether either user blocks the other.
func (r *PostgresRepository) HasBlockBetween(ctx context.Context, userA, userB string) (bool, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, hasBlockBetweenQuery, userA, userB).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

const listQuery = `
SELECT u.id, u.username, u.display_name, u.avatar_url
FROM blocks b
JOIN users u ON u.id = b.blocked_id
WHERE b.blocker_id = $1
ORDER BY u.username ASC, u.id ASC
LIMIT $2
`

// List returns only direct outgoing blocks for the authenticated caller.
func (r *PostgresRepository) List(ctx context.Context, blockerID string, limit int) ([]ListItem, error) {
	rows, err := r.pool.Query(ctx, listQuery, blockerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ListItem, 0, limit)
	for rows.Next() {
		var item ListItem
		if err := rows.Scan(&item.ID, &item.Username, &item.DisplayName, &item.AvatarURL); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
