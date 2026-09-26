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
	Type       string // "image" | "video" | "file" | "voice"
}

// validateUploadedObject confirms an uploaded object belongs to userID, lives
// under one of the allowedDirs (e.g. "uploads", "avatars"), exists in storage,
// and satisfies the media policy. It never trusts the client for type/mime/size
// — those come from storage HeadObject. Reusable by any handler that turns an
// upload into a media record; callers pass the dirs they permit so, e.g., post
// creation can accept "uploads" only and never an avatar key.
//
// Size limits default by namespace: avatars/ uses the avatar policy, other
// dirs the post/story policy. Callers with a stricter context (direct-message
// attachments) use validateUploadedObjectWithLimits.
func (s *Server) validateUploadedObject(ctx context.Context, userID, storageKey string, allowedDirs ...string) (*validatedMedia, error) {
	return s.validateUploadedObjectWithLimits(ctx, userID, storageKey, nil, allowedDirs...)
}

// validateUploadedObjectWithLimits is validateUploadedObject with an explicit
// size policy (nil = the namespace default).
func (s *Server) validateUploadedObjectWithLimits(ctx context.Context, userID, storageKey string, limits *mediaLimits, allowedDirs ...string) (*validatedMedia, error) {
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

	// The dir encodes the storage class, so HeadObject targets the correct
	// bucket (private/ -> private bucket; uploads//avatars/ -> public bucket).
	class := storageClassForKey(storageKey)
	info, err := s.storage.HeadObject(ctx, class, storageKey)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			return nil, errObjectMissing
		}
		return nil, err // internal storage error
	}

	mimeType := normalizedMIME(info.ContentType)
	mediaType, ok := mediaTypeForMime(mimeType)
	if !ok {
		return nil, errUnsupportedMedia
	}
	if info.SizeBytes <= 0 {
		return nil, errUnsupportedMedia
	}
	policy := postMediaLimits
	if dir == "avatars" {
		policy = avatarMediaLimits
	}
	if limits != nil {
		policy = *limits
	}
	if max := policy.limit(mediaType); max <= 0 || info.SizeBytes > max {
		return nil, errUnsupportedMedia
	}

	return &validatedMedia{
		StorageKey: storageKey,
		MimeType:   mimeType,
		SizeBytes:  info.SizeBytes,
		Type:       mediaType,
	}, nil
}

// mediaTypeForMime maps an allowed MIME type to its media category.
func mediaTypeForMime(mime string) (string, bool) {
	mime = normalizedMIME(mime)
	if _, ok := imageMimeExt[mime]; ok {
		return media.TypeImage, true
	}
	if _, ok := videoMimeExt[mime]; ok {
		return media.TypeVideo, true
	}
	if _, ok := fileMimeExt[mime]; ok {
		return "file", true
	}
	if _, ok := voiceMimeExt[mime]; ok {
		return "voice", true
	}
	return "", false
}
