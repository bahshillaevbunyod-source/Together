package conversation

import (
	"context"
	"errors"
	"strings"
	"time"
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
// For group/channel conversations it yields the sender itself when they are a
// participant. $2 is cast explicitly: a bare "SELECT $2" makes Postgres infer
// text for it, which conflicts with the uuid comparisons below and fails the
// statement at prepare time (SQLSTATE 42P08) for every message send.
const otherParticipantQuery = `
SELECT CASE
    WHEN c.type IN ('group', 'channel') THEN (
        SELECT $2::uuid WHERE EXISTS (SELECT 1 FROM conversation_participants p WHERE p.conversation_id=c.id AND p.user_id=$2::uuid)
    )
    WHEN user_low = $2 THEN user_high
    WHEN user_high = $2 THEN user_low
END
FROM conversations c
WHERE c.id = $1
`

const hasBlockBetweenQuery = `
SELECT EXISTS (
    SELECT 1 FROM blocks
    WHERE (blocker_id = $1 AND blocked_id = $2)
       OR (blocker_id = $2 AND blocked_id = $1)
)
`

const insertMessageQuery = `
INSERT INTO messages (conversation_id, sender_id, content)
VALUES ($1, $2, $3)
RETURNING id, conversation_id, sender_id, content, created_at
`

const touchConversationQuery = `UPDATE conversations SET updated_at = now() WHERE id = $1`

// authorizeMessageActor handles the different authorization contracts for
// direct and community conversations. Community messages do not require an
// "other participant"; the current member is sufficient, including for a
// solo community. Direct conversations retain their block and recipient rules.
func authorizeMessageActor(ctx context.Context, tx DBTX, conversationID, senderID string) (string, error) {
	var recipient *string
	if err := tx.QueryRow(ctx, otherParticipantQuery, conversationID, senderID).Scan(&recipient); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotParticipant
		}
		return "", err
	}
	if recipient == nil {
		return "", ErrNotParticipant
	}
	var blocked bool
	if err := tx.QueryRow(ctx, hasBlockBetweenQuery, senderID, *recipient).Scan(&blocked); err != nil {
		return "", err
	}
	if blocked {
		return "", ErrBlocked
	}
	return *recipient, nil
}

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

	recipient, err := authorizeMessageActor(ctx, tx, conversationID, senderID)
	if err != nil {
		return nil, "", err
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
	return &m, recipient, nil
}

const insertMessageAttachmentQuery = `
INSERT INTO message_attachments (message_id, storage_key, filename, type, mime_type, size_bytes)
VALUES ($1, $2, $3, $4, $5, $6)
`

func (r *PostgresRepository) CreateMessageWithAttachment(ctx context.Context, conversationID, senderID, content string, attachment Attachment) (*Message, string, error) {
	content = strings.TrimSpace(content)
	if content != "" && utf8.RuneCountInString(content) > maxMessageContentRunes {
		return nil, "", ErrContentTooLong
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback(ctx)
	recipient, err := authorizeMessageActor(ctx, tx, conversationID, senderID)
	if err != nil {
		return nil, "", err
	}
	if attachment.Filename == "" || attachment.StorageKey == "" || attachment.SizeBytes <= 0 {
		return nil, "", ErrInvalidAttachment
	}
	var m Message
	if err := tx.QueryRow(ctx, `INSERT INTO messages (conversation_id, sender_id, content) VALUES ($1,$2,$3) RETURNING id, conversation_id, sender_id, content, created_at`, conversationID, senderID, content).Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.Content, &m.CreatedAt); err != nil {
		return nil, "", err
	}
	attachment.MessageID = m.ID
	if _, err := tx.Exec(ctx, insertMessageAttachmentQuery, attachment.MessageID, attachment.StorageKey, attachment.Filename, attachment.Type, attachment.MimeType, attachment.SizeBytes); err != nil {
		return nil, "", err
	}
	if _, err := tx.Exec(ctx, touchConversationQuery, conversationID); err != nil {
		return nil, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, "", err
	}
	m.Attachment = &attachment
	return &m, recipient, nil
}

// listMessagesQuery returns a conversation's messages newest-first, keyset
// paginated. conversation_id is a constant here, so there is no per-row lookup.
const listMessagesQuery = `
SELECT m.id, m.sender_id, m.content, m.created_at, m.updated_at, m.deleted_at,
       m.source_language, m.source_language_confidence, m.source_language_resolution,
       a.id, a.storage_key, a.filename, a.type, a.mime_type, a.size_bytes, a.created_at
FROM messages m
LEFT JOIN message_attachments a ON a.message_id = m.id
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
		var attachmentID, attachmentKey, attachmentName, attachmentType, attachmentMime *string
		var attachmentSize *int64
		var attachmentCreated *time.Time
		if err := rows.Scan(
			&m.ID, &m.SenderID, &m.Content, &m.CreatedAt, &m.UpdatedAt, &m.DeletedAt,
			&m.SourceLanguage, &m.SourceLanguageConfidence, &m.SourceLanguageResolution,
			&attachmentID, &attachmentKey, &attachmentName, &attachmentType, &attachmentMime, &attachmentSize, &attachmentCreated,
		); err != nil {
			return nil, err
		}
		if attachmentID != nil && attachmentKey != nil && attachmentName != nil && attachmentType != nil && attachmentMime != nil && attachmentSize != nil && attachmentCreated != nil {
			m.Attachment = &Attachment{ID: *attachmentID, MessageID: m.ID, StorageKey: *attachmentKey, Filename: *attachmentName, Type: *attachmentType, MimeType: *attachmentMime, SizeBytes: *attachmentSize, CreatedAt: *attachmentCreated}
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

const searchMessagesQuery = `
SELECT m.id, m.conversation_id, m.sender_id, m.content, m.created_at, m.updated_at, m.deleted_at,
       m.source_language, m.source_language_confidence, m.source_language_resolution,
       other.id, other.username, other.display_name
FROM messages m
JOIN conversation_participants mine ON mine.conversation_id = m.conversation_id AND mine.user_id = $1
JOIN conversations c ON c.id = m.conversation_id
JOIN users other ON other.id = CASE WHEN c.user_low = $1 THEN c.user_high ELSE c.user_low END
WHERE m.deleted_at IS NULL AND m.content ILIKE '%' || $2 || '%'
ORDER BY m.created_at DESC, m.id DESC
LIMIT $3
`

func (r *PostgresRepository) SearchMessages(ctx context.Context, userID, query string, limit int) ([]SearchResult, error) {
	rows, err := r.q.Query(ctx, searchMessagesQuery, userID, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]SearchResult, 0, limit)
	for rows.Next() {
		var item SearchResult
		if err := rows.Scan(&item.ID, &item.ConversationID, &item.SenderID, &item.Content, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
			&item.SourceLanguage, &item.SourceLanguageConfidence, &item.SourceLanguageResolution,
			&item.OtherID, &item.OtherUsername, &item.OtherDisplayName); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

const updateMessageQuery = `
UPDATE messages
SET content = $3, updated_at = now(), source_language = NULL, source_language_confidence = NULL, source_language_resolution = NULL
WHERE id = $1 AND sender_id = $2 AND deleted_at IS NULL
  AND (length(btrim($3)) > 0 OR EXISTS (SELECT 1 FROM message_attachments a WHERE a.message_id = messages.id))
RETURNING id, conversation_id, sender_id, content, created_at, updated_at, deleted_at
`

func (r *PostgresRepository) UpdateMessage(ctx context.Context, messageID, senderID, content string) (*Message, string, error) {
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
	defer tx.Rollback(ctx)
	var m Message
	if err := tx.QueryRow(ctx, updateMessageQuery, messageID, senderID, content).Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.Content, &m.CreatedAt, &m.UpdatedAt, &m.DeletedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrMessageNotFound
		}
		return nil, "", err
	}
	var attachment Attachment
	if err := tx.QueryRow(ctx, `SELECT id, storage_key, filename, type, mime_type, size_bytes, created_at FROM message_attachments WHERE message_id=$1`, m.ID).Scan(&attachment.ID, &attachment.StorageKey, &attachment.Filename, &attachment.Type, &attachment.MimeType, &attachment.SizeBytes, &attachment.CreatedAt); err == nil {
		attachment.MessageID = m.ID
		m.Attachment = &attachment
	}
	recipient, err := authorizeMessageActor(ctx, tx, m.ConversationID, senderID)
	if err != nil {
		return nil, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, "", err
	}
	return &m, recipient, nil
}

const deleteMessageQuery = `
UPDATE messages SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND sender_id = $2 AND deleted_at IS NULL
RETURNING conversation_id, deleted_at
`

func (r *PostgresRepository) DeleteMessage(ctx context.Context, messageID, senderID string) (string, string, time.Time, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", "", time.Time{}, err
	}
	defer tx.Rollback(ctx)
	var conversationID string
	var deletedAt time.Time
	if err := tx.QueryRow(ctx, deleteMessageQuery, messageID, senderID).Scan(&conversationID, &deletedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", time.Time{}, ErrMessageNotFound
		}
		return "", "", time.Time{}, err
	}
	recipient, err := authorizeMessageActor(ctx, tx, conversationID, senderID)
	if err != nil {
		return "", "", time.Time{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", time.Time{}, err
	}
	return conversationID, recipient, deletedAt, nil
}

const setMessageLanguageMetadataQuery = `
UPDATE messages
SET source_language = $3,
    source_language_confidence = $4,
    source_language_resolution = $5
WHERE id = $1 AND sender_id = $2
`

func (r *PostgresRepository) SetMessageLanguageMetadata(ctx context.Context, messageID, senderID string, sourceLanguage *string, confidence *float64, resolution string) error {
	_, err := r.q.Exec(ctx, setMessageLanguageMetadataQuery, messageID, senderID, sourceLanguage, confidence, resolution)
	return err
}

const recentSenderContextQuery = `
SELECT content
FROM messages
WHERE conversation_id = $1
  AND sender_id = $2
  AND id <> $3
ORDER BY created_at DESC, id DESC
LIMIT 1
`

func (r *PostgresRepository) RecentSenderContext(ctx context.Context, conversationID, senderID, excludeMessageID string) (string, error) {
	var content string
	err := r.q.QueryRow(ctx, recentSenderContextQuery, conversationID, senderID, excludeMessageID).Scan(&content)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return content, nil
}

// listConversationsQuery returns a user's conversations newest-activity first.
// The other participant is the side of the pair that is not the viewer. The
// LEFT JOIN LATERAL fetches each conversation's single latest message in one
// query, so there is no per-row (N+1) lookup.
const listConversationsQuery = `
SELECT c.id, c.updated_at,
       other.id, other.username, other.display_name, other.avatar_url,
       lm.id, lm.sender_id, lm.content, lm.created_at,
       me.muted_at IS NOT NULL,
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
WHERE c.type = 'direct'
  AND ($2::timestamptz IS NULL
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
			&it.Muted, &it.UnreadCount,
		); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

const setConversationMutedQuery = `
UPDATE conversation_participants
SET muted_at = CASE WHEN $3 THEN now() ELSE NULL END
WHERE conversation_id = $1 AND user_id = $2
`

func (r *PostgresRepository) SetConversationMuted(ctx context.Context, conversationID, userID string, muted bool) (bool, error) {
	tag, err := r.q.Exec(ctx, setConversationMutedQuery, conversationID, userID, muted)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
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
