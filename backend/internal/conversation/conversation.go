// Package conversation manages private 1-to-1 conversations between users.
package conversation

import (
	"errors"
	"time"
)

// maxMessageContentRunes bounds a message body (matches the comment limit).
const maxMessageContentRunes = 2000

// Errors returned by the repository.
var (
	// ErrSameUser is returned when a conversation is requested with oneself.
	ErrSameUser = errors.New("conversation: cannot open a conversation with yourself")
	// ErrNotParticipant is returned when the sender is not a conversation member.
	ErrNotParticipant = errors.New("conversation: sender is not a participant")
	// ErrEmptyContent is returned when a message body is blank after trimming.
	ErrEmptyContent = errors.New("conversation: message content is required")
	// ErrContentTooLong is returned when a message exceeds the length limit.
	ErrContentTooLong = errors.New("conversation: message content is too long")
)

// Conversation mirrors a row in the `conversations` table. Members are stored
// as a canonical ordered pair (UserLow < UserHigh).
type Conversation struct {
	ID        string
	UserLow   string
	UserHigh  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Message mirrors a row in the `messages` table.
type Message struct {
	ID             string
	ConversationID string
	SenderID       string
	Content        string
	CreatedAt      time.Time
}

// MessageCursor is a stable keyset position: a message's created_at plus id.
// Ordering is (created_at DESC, id DESC).
type MessageCursor struct {
	CreatedAt time.Time
	ID        string
}

// ConversationCursor is a stable keyset position for a conversation list: the
// conversation's updated_at (latest activity) plus id. Ordering is
// (updated_at DESC, id DESC).
type ConversationCursor struct {
	UpdatedAt time.Time
	ID        string
}

// ListItem is one conversation in a user's list: the other participant's basic
// profile plus the latest message (nil fields when the conversation has none)
// and the latest-activity time used for ordering and the next cursor.
type ListItem struct {
	ID        string
	UpdatedAt time.Time

	OtherID          string
	OtherUsername    string
	OtherDisplayName string
	OtherAvatarURL   *string

	LastMessageID        *string
	LastMessageSenderID  *string
	LastMessageContent   *string
	LastMessageCreatedAt *time.Time

	// UnreadCount is the number of messages from the other participant that the
	// viewer has not yet read.
	UnreadCount int64
}
