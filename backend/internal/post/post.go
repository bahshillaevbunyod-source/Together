// Package post holds the Post domain model and persistence.
package post

import (
	"errors"
	"time"
)

// ErrNotFound is returned when a post does not exist.
var ErrNotFound = errors.New("post not found")

// Visibility values allowed by the posts_visibility_valid constraint.
const (
	VisibilityPublic    = "public"
	VisibilityFollowers = "followers"
	VisibilityPrivate   = "private"
)

// Post mirrors a row in the `posts` table.
type Post struct {
	ID         string
	AuthorID   string
	Content    *string // nullable: a post may be media-only
	Visibility string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// CreateInput carries the validated fields to create a post. AuthorID always
// comes from the authenticated user, never from the request body.
type CreateInput struct {
	AuthorID   string
	Content    *string // nullable: media-only posts have no text
	Visibility string
}

// PostUpdate is a validated partial update of a post. A nil field is left
// unchanged. author_id / created_at / id are never updatable.
type PostUpdate struct {
	Content    *string
	Visibility *string
}

// HasChanges reports whether at least one field will be updated.
func (u PostUpdate) HasChanges() bool {
	return u.Content != nil || u.Visibility != nil
}

// Cursor is a stable keyset position: a post's created_at plus its id.
// Ordering is (created_at DESC, id DESC).
type Cursor struct {
	CreatedAt time.Time
	ID        string
}

// FeedItem is a post joined with its author's public-safe fields.
type FeedItem struct {
	ID                string
	AuthorID          string
	Content           *string // nullable: a post may be media-only
	Visibility        string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	AuthorUsername    string
	AuthorDisplayName string
	AuthorAvatarURL   *string
	LikesCount        int64
	LikedByMe         bool
	CommentsCount     int64
}
