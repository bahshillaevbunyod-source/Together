// Package story manages ephemeral stories and their per-viewer view state.
//
// Access rule (the single source of truth for who may see a story): a viewer may
// see a story only when the viewer is the author OR has an accepted canonical
// `follows` edge to the author, AND no block exists in either direction. Pending
// follow_requests never grant access — this package never references that table.
// A story is "active" only for 24 hours after created_at; every read path
// enforces that window (there is no cleanup job).
package story

import (
	"errors"
	"time"
)

// ErrNotFound is returned when a story does not exist, has expired, or is not
// visible to the viewer (the three are intentionally indistinguishable to
// callers so access is never leaked).
var ErrNotFound = errors.New("story not found")

// Media type values allowed by the stories_type_valid constraint.
const (
	TypeImage = "image"
	TypeVideo = "video"
)

// Story mirrors a row in the `stories` table (no author join, no view state).
type Story struct {
	ID         string
	AuthorID   string
	Type       string
	StorageKey string
	MimeType   string
	Width      *int
	Height     *int
	DurationMs *int64
	CreatedAt  time.Time
}

// CreateInput carries the validated fields to create a story. AuthorID always
// comes from the authenticated user, never from the request body.
type CreateInput struct {
	AuthorID   string
	Type       string
	StorageKey string
	MimeType   string
	Width      *int
	Height     *int
	DurationMs *int64
}

// Cursor is a stable keyset position: a story's created_at plus its id.
// Ordering is (created_at DESC, id DESC).
type Cursor struct {
	CreatedAt time.Time
	ID        string
}

// Item is an active story joined with its author's public-safe fields and the
// viewer's viewed/unviewed state. Returned by the visible get/list paths.
type Item struct {
	ID                string
	AuthorID          string
	Type              string
	StorageKey        string
	MimeType          string
	Width             *int
	Height            *int
	DurationMs        *int64
	CreatedAt         time.Time
	AuthorUsername    string
	AuthorDisplayName string
	AuthorAvatarURL   *string
	Viewed            bool
}
