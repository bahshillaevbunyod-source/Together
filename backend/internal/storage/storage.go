// Package storage abstracts object storage (S3 / Cloudflare R2) for media.
// This is the interface only — no SDK or concrete implementation yet.
package storage

import (
	"context"
	"errors"
	"time"
)

// ErrObjectNotFound is returned when an object does not exist.
var ErrObjectNotFound = errors.New("object not found")

// ObjectInfo is the metadata of a stored object.
type ObjectInfo struct {
	ContentType string
	SizeBytes   int64
}

// PresignedUpload is the result of requesting a direct upload URL. The client
// PUTs the object to UploadURL; the URL is valid until ExpiresAt.
type PresignedUpload struct {
	UploadURL string
	ExpiresAt time.Time
}

// Repository abstracts object storage operations.
type Repository interface {
	// CreateUploadURL returns a presigned URL for a direct client upload of an
	// object at key, constrained to the given mimeType and sizeBytes.
	CreateUploadURL(ctx context.Context, key, mimeType string, sizeBytes int64) (*PresignedUpload, error)
	// DeleteObject removes the object at key.
	DeleteObject(ctx context.Context, key string) error
	// HeadObject returns metadata for the object at key, or ErrObjectNotFound.
	HeadObject(ctx context.Context, key string) (*ObjectInfo, error)
}
