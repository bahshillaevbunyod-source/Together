package server

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"together/backend/internal/media"
	"together/backend/internal/storage"
)

// Reasons the upload validation can fail. Callers may branch on these.
var (
	errInvalidStorageKey = errors.New("invalid storage key")
	errObjectMissing     = errors.New("uploaded object not found")
	errUnsupportedMedia  = errors.New("unsupported media type or size")
)

// validatedMedia is a confirmed, policy-compliant uploaded object.
type validatedMedia struct {
	StorageKey string
	MimeType   string
	SizeBytes  int64
	Type       string // "image" | "video"
}

// validateUploadedObject confirms an uploaded object belongs to userID, lives
// under one of the allowedDirs (e.g. "uploads", "avatars"), exists in storage,
// and satisfies the media policy. It never trusts the client for type/mime/size
// — those come from storage HeadObject. Reusable by any handler that turns an
// upload into a media record; callers pass the dirs they permit so, e.g., post
// creation can accept "uploads" only and never an avatar key.
func (s *Server) validateUploadedObject(ctx context.Context, userID, storageKey string, allowedDirs ...string) (*validatedMedia, error) {
	base := fmt.Sprintf("users/%s/", userID)
	if !strings.HasPrefix(storageKey, base) {
		return nil, errInvalidStorageKey
	}
	// Reject traversal / escaping the prefix.
	if strings.Contains(storageKey, "..") || strings.Contains(storageKey, `\`) {
		return nil, errInvalidStorageKey
	}
	// The remainder must be exactly "<dir>/<file>": an allowed dir and a single
	// file segment (no nesting).
	rest := storageKey[len(base):]
	slash := strings.IndexByte(rest, '/')
	if slash <= 0 {
		return nil, errInvalidStorageKey
	}
	dir := rest[:slash]
	file := rest[slash+1:]
	if file == "" || strings.Contains(file, "/") {
		return nil, errInvalidStorageKey
	}
	allowed := false
	for _, d := range allowedDirs {
		if dir == d {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, errInvalidStorageKey
	}

	info, err := s.storage.HeadObject(ctx, storageKey)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			return nil, errObjectMissing
		}
		return nil, err // internal storage error
	}

	mediaType, ok := mediaTypeForMime(info.ContentType)
	if !ok {
		return nil, errUnsupportedMedia
	}
	if info.SizeBytes <= 0 {
		return nil, errUnsupportedMedia
	}
	if mediaType == media.TypeImage && info.SizeBytes > maxImageBytes {
		return nil, errUnsupportedMedia
	}
	if mediaType == media.TypeVideo && info.SizeBytes > maxVideoBytes {
		return nil, errUnsupportedMedia
	}

	return &validatedMedia{
		StorageKey: storageKey,
		MimeType:   info.ContentType,
		SizeBytes:  info.SizeBytes,
		Type:       mediaType,
	}, nil
}

// mediaTypeForMime maps an allowed MIME type to its media category.
func mediaTypeForMime(mime string) (string, bool) {
	if _, ok := imageMimeExt[mime]; ok {
		return media.TypeImage, true
	}
	if _, ok := videoMimeExt[mime]; ok {
		return media.TypeVideo, true
	}
	return "", false
}
