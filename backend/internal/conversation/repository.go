package conversation

import (
	"context"
	"time"
)

// Repository abstracts conversation persistence so handlers never touch SQL.
type Repository interface {
	// OpenPrivateConversation returns the private conversation between userA and
	// userB, creating it (and both participant rows) if it does not yet exist.
	// The pair is canonicalized, so the argument order does not matter and
	// repeated calls never create duplicate conversations. Returns ErrSameUser
	// when userA == userB.
	OpenPrivateConversation(ctx context.Context, userA, userB string) (*Conversation, error)
	// CreateMessage inserts a message sent by senderID into conversationID and
	// bumps the conversation's updated_at, atomically. The sender must be a
	// participant. Content is trimmed; it must be non-empty and within the
	// length limit. Returns the stored message and the other participant's id
	// (the recipient), both derived server-side.
	CreateMessage(ctx context.Context, conversationID, senderID, content string) (*Message, string, error)
	// SetMessageLanguageMetadata persists source-language metadata only for the message sender.
	SetMessageLanguageMetadata(
		ctx context.Context,
		messageID string,
		senderID string,
		sourceLanguage *string,
		confidence *float64,
		resolution string,
	) error
	// RecentSenderContext returns one newest prior message from the same sender.
	RecentSenderContext(
		ctx context.Context,
		conversationID string,
		senderID string,
		excludeMessageID string,
	) (string, error)
	// ListMessages returns up to `limit` messages in conversationID, newest
	// first, keyset-paginated by the cursor (nil for the first page). userID
	// must be a participant; otherwise ErrNotParticipant is returned.
	ListMessages(ctx context.Context, conversationID, userID string, cur *MessageCursor, limit int) ([]Message, error)
	SearchMessages(ctx context.Context, userID, query string, limit int) ([]SearchResult, error)
	UpdateMessage(ctx context.Context, messageID, senderID, content string) (*Message, string, error)
	DeleteMessage(ctx context.Context, messageID, senderID string) (conversationID, recipientID string, deletedAt time.Time, err error)
	// ListConversations returns up to `limit` of userID's conversations, newest
	// activity first, keyset-paginated by the cursor (nil for the first page).
	// Each item carries the other participant's basic profile and the latest
	// message, if any.
	ListConversations(ctx context.Context, userID string, cur *ConversationCursor, limit int) ([]ListItem, error)
	// MarkConversationRead marks userID's read state in conversationID up to the
	// latest message. It reports whether userID is a participant (false means no
	// such membership). Idempotent.
	MarkConversationRead(ctx context.Context, conversationID, userID string) (bool, error)
	SetConversationMuted(ctx context.Context, conversationID, userID string, muted bool) (bool, error)
}
