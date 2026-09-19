// Package followrequest manages pending follow requests for private accounts.
//
// A pending request is intentionally SEPARATE from the `follows` graph: a row
// here never grants follower access. Accepting a request deletes its row here
// and inserts the canonical edge into `follows` (done transactionally by
// followrequestservice); declining or cancelling only deletes the row here.
package followrequest

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX is the minimal executor both *pgxpool.Pool and pgx.Tx satisfy, so a
// pending request can be deleted inside the accept transaction alongside the
// follow-edge insert and its notification.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Cursor is a stable keyset position: the request's created_at plus the
// requester's id. Ordering is (created_at DESC, requester_id DESC), matching
// the followers/following list convention.
type Cursor struct {
	CreatedAt time.Time
	UserID    string
}

// ListItem is one public-safe requester in a private account's incoming
// request list. CreatedAt is the request time, used only to build the next
// cursor.
type ListItem struct {
	ID             string
	Username       string
	DisplayName    string
	AvatarURL      *string
	CountryCode    *string
	City           *string
	NativeLanguage string
	CreatedAt      time.Time
}

// Repository abstracts pending follow-request persistence so handlers never
// touch SQL.
type Repository interface {
	// Create records a pending request from requesterID to targetID. Idempotent:
	// a duplicate request reports created=false and adds no row.
	Create(ctx context.Context, requesterID, targetID string) (bool, error)
	// CreateTx records a pending request using the given executor so it can share
	// a transaction with its notification. Reports whether a new row was created.
	CreateTx(ctx context.Context, q DBTX, requesterID, targetID string) (bool, error)
	// Exists reports whether a pending request from requesterID to targetID
	// currently exists.
	Exists(ctx context.Context, requesterID, targetID string) (bool, error)
	// Delete removes a pending request (used for cancel by the requester and
	// decline by the target — the same row either way). Reports whether a row
	// existed. Deleting a missing request is a no-op.
	Delete(ctx context.Context, requesterID, targetID string) (bool, error)
	// DeleteTx removes a pending request using the given executor so it can
	// share the accept transaction. Reports whether a row existed.
	DeleteTx(ctx context.Context, q DBTX, requesterID, targetID string) (bool, error)
	// CountIncoming returns how many pending requests target userID.
	CountIncoming(ctx context.Context, userID string) (int64, error)
	// ListIncoming returns up to `limit` requesters with a pending request to
	// userID, ordered by the keyset cursor. Requesters in a block relationship
	// with userID (either direction) are excluded, mirroring the followers/
	// following lists.
	ListIncoming(ctx context.Context, userID string, cur *Cursor, limit int) ([]ListItem, error)
}
