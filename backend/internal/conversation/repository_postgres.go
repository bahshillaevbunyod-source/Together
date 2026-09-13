package conversation

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBTX is the minimal executor both *pgxpool.Pool and pgx.Tx satisfy.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Tx is a transaction usable as a query executor.
type Tx interface {
	DBTX
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// Beginner opens transactions.
type Beginner interface {
	Begin(ctx context.Context) (Tx, error)
}

// queryer is the non-transactional read/write path *pgxpool.Pool satisfies.
type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// PostgresRepository is the production Repository backed by pgxpool.
type PostgresRepository struct {
	db Beginner
	q  queryer
}

// Compile-time assurance that the production repository satisfies Repository.
var _ Repository = (*PostgresRepository)(nil)

// NewPostgresRepository builds a Repository over the given connection pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: poolBeginner{pool: pool}, q: pool}
}

// upsertConversationQuery inserts the canonical pair, or returns the existing
// conversation on conflict. The no-op DO UPDATE lets RETURNING yield the row
// whether it was just inserted or already existed.
const upsertConversationQuery = `
INSERT INTO conversations (user_low, user_high)
VALUES ($1, $2)
ON CONFLICT (user_low, user_high)
    DO UPDATE SET updated_at = conversations.updated_at
RETURNING id, user_low, user_high, created_at, updated_at
`

// insertParticipantsQuery creates both participant rows, ignoring any that
// already exist (idempotent under repeated opens).
const insertParticipantsQuery = `
INSERT INTO conversation_participants (conversation_id, user_id)
VALUES ($1, $2), ($1, $3)
ON CONFLICT DO NOTHING
`

// OpenPrivateConversation returns the conversation between userA and userB,
// creating it and both participant rows atomically if needed. The pair is
// canonicalized (lower id first) and the unique constraint on (user_low,
// user_high) guarantees no duplicate conversation under repeated or concurrent
// requests.
func (r *PostgresRepository) OpenPrivateConversation(ctx context.Context, userA, userB string) (*Conversation, error) {
	if userA == userB {
		return nil, ErrSameUser
	}
	low, high := userA, userB
	if low > high {
		low, high = high, low
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) // no-op after a successful Commit

	var c Conversation
	if err := tx.QueryRow(ctx, upsertConversationQuery, low, high).Scan(
		&c.ID, &c.UserLow, &c.UserHigh, &c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, insertParticipantsQuery, c.ID, low, high); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &c, nil
}

const isParticipantQuery = `
SELECT EXISTS (
    SELECT 1 FROM conversation_participants
    WHERE conversation_id = $1 AND user_id = $2
)
`

// otherParticipantQuery returns the other member of the conversation for a
// given sender. It yields NULL when the sender is not part of the pair, and no
// row when the conversation does not exist — both mean "not a participant".
const otherParticipantQuery = `
SELECT CASE
    WHEN user_low = $2 THEN user_high
    WHEN user_high = $2 THEN user_low
END
FROM conversations
WHERE id = $1
`

const insertMessageQuery = `
INSERT INTO messages (conversation_id, sender_id, content)
VALUES ($1, $2, $3)
RETURNING id, conversation_id, sender_id, content, created_at
`

const touchConversationQuery = `UPDATE conversations SET updated_at = now() WHERE id = $1`

// CreateMessage validates and stores a message, then bumps the conversation's
// updated_at, all in one transaction. The sender must be a participant. It
// returns the stored message and the recipient (the other participant),
// resolved server-side from the conversation pair.
func (r *PostgresRepository) CreateMessage(ctx context.Context, conversationID, senderID, content string) (*Message, string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, "", ErrEmptyContent
	}
	if utf8.RuneCountInString(content) > maxMessageContentRunes {
		return nil, "", ErrContentTooLong
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback(ctx) // no-op after a successful Commit

	var recipient *string
	if err := tx.QueryRow(ctx, otherParticipantQuery, conversationID, senderID).Scan(&recipient); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrNotParticipant
		}
		return nil, "", err
	}
	if recipient == nil {
		return nil, "", ErrNotParticipant
	}

	var m Message
	if err := tx.QueryRow(ctx, insertMessageQuery, conversationID, senderID, content).Scan(
		&m.ID, &m.ConversationID, &m.SenderID, &m.Content, &m.CreatedAt,
	); err != nil {
		return nil, "", err
	}

	if _, err := tx.Exec(ctx, touchConversationQuery, conversationID); err != nil {
		return nil, "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, "", err
	}
	return &m, *recipient, nil
}

// listMessagesQuery returns a conversation's messages newest-first, keyset
// paginated. conversation_id is a constant here, so there is no per-row lookup.
const listMessagesQuery = `
SELECT m.id, m.sender_id, m.content, m.created_at
FROM messages m
WHERE m.conversation_id = $1
  AND ($2::timestamptz IS NULL
       OR m.created_at < $2
       OR (m.created_at = $2 AND m.id < $3::uuid))
ORDER BY m.created_at DESC, m.id DESC
LIMIT $4
`

// ListMessages returns up to `limit` messages in a conversation, newest first,
// after verifying userID is a participant. One membership check plus one list
// query — no per-row queries.
func (r *PostgresRepository) ListMessages(ctx context.Context, conversationID, userID string, cur *MessageCursor, limit int) ([]Message, error) {
	var isParticipant bool
	if err := r.q.QueryRow(ctx, isParticipantQuery, conversationID, userID).Scan(&isParticipant); err != nil {
		return nil, err
	}
	if !isParticipant {
		return nil, ErrNotParticipant
	}

	var ts, id any
	if cur != nil {
		ts = cur.CreatedAt
		id = cur.ID
	}

	rows, err := r.q.Query(ctx, listMessagesQuery, conversationID, ts, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Message, 0, limit)
	for rows.Next() {
		m := Message{ConversationID: conversationID}
		if err := rows.Scan(&m.ID, &m.SenderID, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

// listConversationsQuery returns a user's conversations newest-activity first.
// The other participant is the side of the pair that is not the viewer. The
// LEFT JOIN LATERAL fetches each conversation's single latest message in one
// query, so there is no per-row (N+1) lookup.
const listConversationsQuery = `
SELECT c.id, c.updated_at,
       other.id, other.username, other.display_name, other.avatar_url,
       lm.id, lm.sender_id, lm.content, lm.created_at,
       (SELECT count(*) FROM messages um
         WHERE um.conversation_id = c.id
           AND um.sender_id <> $1
           AND (me.last_read_at IS NULL OR um.created_at > me.last_read_at)
       ) AS unread_count
FROM conversations c
JOIN conversation_participants me
    ON me.conversation_id = c.id AND me.user_id = $1
JOIN users other
    ON other.id = CASE WHEN c.user_low = $1 THEN c.user_high ELSE c.user_low END
LEFT JOIN LATERAL (
    SELECT m.id, m.sender_id, m.content, m.created_at
    FROM messages m
    WHERE m.conversation_id = c.id
    ORDER BY m.created_at DESC, m.id DESC
    LIMIT 1
) lm ON true
WHERE ($2::timestamptz IS NULL
       OR c.updated_at < $2
       OR (c.updated_at = $2 AND c.id < $3::uuid))
ORDER BY c.updated_at DESC, c.id DESC
LIMIT $4
`

// ListConversations returns up to `limit` of a user's conversations, newest
// activity first, with the other participant and latest message per row.
func (r *PostgresRepository) ListConversations(ctx context.Context, userID string, cur *ConversationCursor, limit int) ([]ListItem, error) {
	var ts, id any
	if cur != nil {
		ts = cur.UpdatedAt
		id = cur.ID
	}

	rows, err := r.q.Query(ctx, listConversationsQuery, userID, ts, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ListItem, 0, limit)
	for rows.Next() {
		var it ListItem
		if err := rows.Scan(
			&it.ID, &it.UpdatedAt,
			&it.OtherID, &it.OtherUsername, &it.OtherDisplayName, &it.OtherAvatarURL,
			&it.LastMessageID, &it.LastMessageSenderID, &it.LastMessageContent, &it.LastMessageCreatedAt,
			&it.UnreadCount,
		); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// markReadQuery advances the viewer's read markers to the conversation's latest
// message. It touches only the caller's participant row, so it is idempotent
// and never affects the other member. RowsAffected reports membership.
const markReadQuery = `
UPDATE conversation_participants p
SET last_read_message_id = (
        SELECT m.id FROM messages m
        WHERE m.conversation_id = p.conversation_id
        ORDER BY m.created_at DESC, m.id DESC
        LIMIT 1
    ),
    last_read_at = now()
WHERE p.conversation_id = $1 AND p.user_id = $2
`

// MarkConversationRead marks the caller's read state up to the latest message.
// Returns whether the caller is a participant (RowsAffected > 0).
func (r *PostgresRepository) MarkConversationRead(ctx context.Context, conversationID, userID string) (bool, error) {
	tag, err := r.q.Exec(ctx, markReadQuery, conversationID, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// poolBeginner adapts *pgxpool.Pool to Beginner (pgx.Tx satisfies Tx).
type poolBeginner struct {
	pool *pgxpool.Pool
}

func (b poolBeginner) Begin(ctx context.Context) (Tx, error) {
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return tx, nil
}
