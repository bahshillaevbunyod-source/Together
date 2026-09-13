// Package comment holds post comments and their persistence.
package comment

import (
	"errors"
	"time"
)

// ErrNotFound is returned when a comment does not exist.
var ErrNotFound = errors.New("comment not found")

// Comment mirrors a row in the `post_comments` table.
type Comment struct {
	ID        string
	PostID    string
	AuthorID  string
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateInput carries the validated fields to create a comment. AuthorID
// always comes from the authenticated user, never from the request body.
type CreateInput struct {
	PostID   string
	AuthorID string
	Content  string
}

// Cursor is a stable keyset position: a comment's created_at plus its id.
// Ordering is (created_at DESC, id DESC).
type Cursor struct {
	CreatedAt time.Time
	ID        string
}

// ListItem is a comment joined with its author's public-safe fields.
type ListItem struct {
	ID                string
	AuthorID          string
	Content           string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	AuthorUsername    string
	AuthorDisplayName string
	AuthorAvatarURL   *string
}
