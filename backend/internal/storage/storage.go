// Package storage abstracts object storage (S3 / Cloudflare R2) for media.
// It routes operations to one of two storage classes: a PUBLIC bucket
// (avatars + public post media, served by a permanent public URL) and a
// PRIVATE bucket (followers-only / private post media, never publicly
// reachable — read only via short-lived presigned GET URLs).
package storage

import (
	"context"
	"errors"
	"time"
)

// ErrObjectNotFound is returned when an object does not exist.
var ErrObjectNotFound = errors.New("object not found")

// ErrClassNotConfigured is returned when an operation targets a storage class
// that has no bucket/credentials configured (e.g. private storage in a dev
// setup without the private bucket env).
var ErrClassNotConfigured = errors.New("storage class not configured")

// Class selects which bucket an operation targets. Callers derive it from the
// server-controlled object key namespace, never from client input.
type Class string

const (
	// ClassPublic is the public bucket: avatars and public post media. Objects
	// are reachable at a stable public URL.
	ClassPublic Class = "public"
	// ClassPrivate is the private bucket: followers-only / private post media.
	// Objects have no public URL and are read only via presigned GET.
	ClassPrivate Class = "private"
)

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

// Repository abstracts object storage operations across storage classes. Every
// method takes the target Class; the bucket is resolved server-side.
type Repository interface {
	// CreateUploadURL returns a presigned URL for a direct client upload of an
	// object at key in the given class, constrained to mimeType and sizeBytes.
	CreateUploadURL(ctx context.Context, class Class, key, mimeType string, sizeBytes int64) (*PresignedUpload, error)
	// PresignGetURL returns a short-lived presigned GET URL for the object at
	// key in the given class. Used to serve private media after authorization.
	PresignGetURL(ctx context.Context, class Class, key string, expiry time.Duration) (string, error)
	// DeleteObject removes the object at key in the given class.
	DeleteObject(ctx context.Context, class Class, key string) error
	// HeadObject returns metadata for the object at key in the given class, or
	// ErrObjectNotFound.
	HeadObject(ctx context.Context, class Class, key string) (*ObjectInfo, error)
	// Configured reports whether the given storage class has a bucket wired up.
	Configured(class Class) bool
}
