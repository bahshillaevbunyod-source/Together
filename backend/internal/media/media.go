// Package media holds post media metadata and its persistence. It stores only
// a storage_key; converting that to a public URL is a caller concern and the
// key is never exposed in API responses.
package media

import (
	"errors"
	"time"
)

// ErrMediaAlreadyAttached is returned when a storage_key is already attached to
// a post (violates the post_media_storage_key_unique constraint).
var ErrMediaAlreadyAttached = errors.New("media already attached")

// Media type values allowed by the post_media_type_valid constraint.
const (
	TypeImage = "image"
	TypeVideo = "video"
)

// Media mirrors a row in the `post_media` table.
type Media struct {
	ID         string
	PostID     string
	Type       string
	StorageKey string
	MimeType   string
	SizeBytes  int64
	Width      *int
	Height     *int
	DurationMs *int64
	SortOrder  int
	CreatedAt  time.Time
}

// CreateInput carries the fields to persist a media row.
type CreateInput struct {
	PostID     string
	Type       string
	StorageKey string
	MimeType   string
	SizeBytes  int64
	Width      *int
	Height     *int
	DurationMs *int64
	SortOrder  int
}
