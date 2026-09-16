package server

import (
	"context"
	"strings"
	"time"

	"together/backend/internal/media"
	"together/backend/internal/storage"
)

// privateGetExpiry is how long a presigned GET URL for private media stays
// valid. Short-lived so revoked access (unfollow / block / visibility change)
// stops working promptly.
const privateGetExpiry = 10 * time.Minute

// mediaResponse is the public shape of a media item. storage_key is never
// exposed; only a derived URL is returned (a permanent public URL for public
// media, or a short-lived presigned URL for private media).
type mediaResponse struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	URL        string `json:"url"`
	MimeType   string `json:"mimeType"`
	Width      *int   `json:"width"`
	Height     *int   `json:"height"`
	DurationMs *int64 `json:"durationMs"`
	SortOrder  int    `json:"sortOrder"`
}

// mediaURL converts a storage_key into a public URL using the configured base.
func mediaURL(base, storageKey string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(storageKey, "/")
}

// storageKeyFromURL is the inverse of mediaURL: it recovers the storage key from
// a public URL that was produced with the given base. It returns "" when the URL
// is not one of ours (e.g. an external avatar URL), so callers can safely skip
// storage cleanup for anything they did not mint.
func storageKeyFromURL(base, url string) string {
	prefix := strings.TrimRight(base, "/") + "/"
	if !strings.HasPrefix(url, prefix) {
		return ""
	}
	return url[len(prefix):]
}

// storageClassForKey derives the storage class from the server-controlled key
// namespace: users/{id}/private/<file> lives in the private bucket; everything
// else (uploads/, avatars/, and any legacy key) is public. The key is always
// server-generated and validated, never taken from the client, so this mapping
// is a reliable source of truth without a schema change.
func storageClassForKey(key string) storage.Class {
	parts := strings.SplitN(key, "/", 4) // ["users", id, dir, file...]
	if len(parts) >= 3 && parts[0] == "users" && parts[2] == "private" {
		return storage.ClassPrivate
	}
	return storage.ClassPublic
}

// toMediaResponses maps media rows to their public shape. Public media get a
// permanent public URL; private media get a short-lived presigned GET URL. It is
// only ever called while building a response for a post the viewer is already
// authorized to see (feed/discover/single-post all resolve access first), so a
// signed URL is never produced for an unauthorized viewer. It always returns a
// non-nil slice so JSON serializes an empty list as [] (never null).
func (s *Server) toMediaResponses(ctx context.Context, items []media.Media) []mediaResponse {
	out := make([]mediaResponse, 0, len(items))
	for _, m := range items {
		out = append(out, mediaResponse{
			ID:         m.ID,
			Type:       m.Type,
			URL:        s.mediaURLForKey(ctx, m.StorageKey),
			MimeType:   m.MimeType,
			Width:      m.Width,
			Height:     m.Height,
			DurationMs: m.DurationMs,
			SortOrder:  m.SortOrder,
		})
	}
	return out
}

// mediaURLForKey returns the delivery URL for a single object: a permanent
// public URL for public/legacy media, or a short-lived presigned GET URL for
// private media. On a presign failure it returns "" rather than leaking a
// public URL for a private object.
func (s *Server) mediaURLForKey(ctx context.Context, key string) string {
	if storageClassForKey(key) == storage.ClassPrivate {
		url, err := s.storage.PresignGetURL(ctx, storage.ClassPrivate, key, privateGetExpiry)
		if err != nil {
			return ""
		}
		return url
	}
	return mediaURL(s.cfg.MediaPublicBaseURL, key)
}
