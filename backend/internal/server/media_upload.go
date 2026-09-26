package server

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"time"

	"together/backend/internal/media"
	"together/backend/internal/post"
	"together/backend/internal/storage"
)

// Media size ceilings are contextual (binary MiB). Bytes always go browser ->
// storage via presigned PUT (which binds the approved Content-Length), so no
// app/proxy request body ever carries the file.
const (
	maxUploadBodyBytes = 1 << 20 // 1 MiB (metadata only)
	// Post and story media.
	maxPostImageBytes = 100 << 20
	maxPostVideoBytes = 250 << 20
	// Profile photos only (purpose "avatar" / the avatars/ namespace).
	maxAvatarImageBytes = 60 << 20
	// Direct-message attachments keep their original ceilings.
	maxImageBytes = 15 << 20
	maxVideoBytes = 200 << 20
	maxFileBytes  = 25 << 20
)

// mediaLimits is the per-type size policy for one upload context. A zero limit
// means that media type is not allowed in the context.
type mediaLimits struct {
	Image, Video, File, Voice int64
}

var (
	// postMediaLimits applies to posts and stories.
	postMediaLimits = mediaLimits{Image: maxPostImageBytes, Video: maxPostVideoBytes, File: maxFileBytes, Voice: maxFileBytes}
	// avatarMediaLimits: profile photos are images only.
	avatarMediaLimits = mediaLimits{Image: maxAvatarImageBytes}
	// messageMediaLimits applies to direct-message attachments (unchanged).
	messageMediaLimits = mediaLimits{Image: maxImageBytes, Video: maxVideoBytes, File: maxFileBytes, Voice: maxFileBytes}
)

// limit returns the ceiling for a media type (0 = not allowed).
func (l mediaLimits) limit(mediaType string) int64 {
	switch mediaType {
	case media.TypeImage:
		return l.Image
	case media.TypeVideo:
		return l.Video
	case "file":
		return l.File
	case "voice":
		return l.Voice
	}
	return 0
}

// limitsForPurpose maps an upload-url purpose to its size policy.
func limitsForPurpose(purpose string) mediaLimits {
	switch purpose {
	case "avatar":
		return avatarMediaLimits
	case "message":
		return messageMediaLimits
	default: // "", "post", "story"
		return postMediaLimits
	}
}

// allowed mime -> safe file extension, per media type.
var imageMimeExt = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
}

var videoMimeExt = map[string]string{
	"video/mp4":  "mp4",
	"video/webm": "webm",
	// iPhone camera default container (.mov). Played natively by Safari and by
	// Chromium when the codec is H.264; HEVC playback depends on the device.
	"video/quicktime": "mov",
}

var fileMimeExt = map[string]string{
	"application/pdf": "pdf",
	"text/plain":      "txt",
	"text/csv":        "csv",
	"application/zip": "zip",
}

var voiceMimeExt = map[string]string{
	"audio/webm": "webm",
	"audio/ogg":  "ogg",
	"audio/mp4":  "m4a",
}

type uploadURLRequest struct {
	Type      string `json:"type"`
	MimeType  string `json:"mimeType"`
	SizeBytes int64  `json:"sizeBytes"`
	// Purpose selects the storage namespace and size policy: "" or "post" ->
	// post media, "story" -> private story media, "message" -> direct-message
	// attachment, "avatar" -> avatars/ (profile photos). Kept optional for
	// backward compatibility (existing post clients send no purpose).
	Purpose string `json:"purpose"`
	// Visibility is the intended post visibility for post uploads. It selects
	// the storage class/bucket: "" or "public" -> public bucket (uploads/);
	// "followers"/"private" -> private bucket (private/). Ignored for avatars
	// (always public). Optional for backward compatibility (defaults public).
	Visibility string `json:"visibility"`
}

// uploadTarget resolves where an upload lives: its storage subdirectory (which
// also encodes the class) and the storage class/bucket. Server-controlled — the
// client only hints intended purpose/visibility; it never picks the bucket.
func uploadTarget(purpose, visibility string) (dir string, class storage.Class, ok bool) {
	switch purpose {
	case "avatar":
		// Avatars are always public and unaffected by post visibility.
		return "avatars", storage.ClassPublic, true
	case "story":
		// Stories are always private media.
		return "private", storage.ClassPrivate, true
	case "", "post", "message":
		switch visibility {
		case "", post.VisibilityPublic:
			return "uploads", storage.ClassPublic, true
		case post.VisibilityFollowers, post.VisibilityPrivate:
			return "private", storage.ClassPrivate, true
		default:
			return "", "", false
		}
	default:
		return "", "", false
	}
}

type uploadURLResponse struct {
	UploadURL  string `json:"uploadUrl"`
	StorageKey string `json:"storageKey"`
	PublicURL  string `json:"publicUrl"`
	ExpiresAt  string `json:"expiresAt"`
}

func (s *Server) handleCreateUploadURL(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "content-type must be application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req uploadURLRequest
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SizeBytes <= 0 {
		writeError(w, http.StatusBadRequest, "sizeBytes must be positive")
		return
	}

	// The extension is derived from the (validated) mime type — never from the
	// client — and the size limit depends on the media type.
	ext, ok := allowedUploadExtension(req.Purpose, req.Type, req.MimeType, req.SizeBytes)
	if !ok {
		writeError(w, http.StatusBadRequest, "unsupported media type or size")
		return
	}

	dir, class, ok := uploadTarget(req.Purpose, req.Visibility)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid purpose or visibility")
		return
	}
	if !s.storage.Configured(class) {
		writeError(w, http.StatusServiceUnavailable, "storage not available")
		return
	}

	id, err := newUUIDv4()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Key is fully server-generated: user id from auth context, namespace dir
	// (which encodes the storage class), uuid, safe ext.
	key := fmt.Sprintf("users/%s/%s/%s.%s", me.ID, dir, id, ext)

	upload, err := s.storage.CreateUploadURL(r.Context(), class, key, req.MimeType, req.SizeBytes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Private media has no permanent public URL; it is read only via presigned
	// GET after authorization. Only public media exposes a stable public URL.
	publicURL := ""
	if class == storage.ClassPublic {
		publicURL = mediaURL(s.cfg.MediaPublicBaseURL, key)
	}

	writeJSON(w, http.StatusOK, uploadURLResponse{
		UploadURL:  upload.UploadURL,
		StorageKey: key,
		PublicURL:  publicURL,
		ExpiresAt:  upload.ExpiresAt.Format(time.RFC3339),
	})
}

type confirmUploadRequest struct {
	StorageKey string `json:"storageKey"`
}

type confirmUploadResponse struct {
	StorageKey string `json:"storageKey"`
	Type       string `json:"type"`
	MimeType   string `json:"mimeType"`
	SizeBytes  int64  `json:"sizeBytes"`
	PublicURL  string `json:"publicUrl"`
}

func (s *Server) handleConfirmUpload(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "content-type must be application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req confirmUploadRequest
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Confirm accepts any post/avatar namespace (public uploads, private post
	// media, or avatars); the key's dir determines the bucket.
	confirmed, err := s.validateUploadedObject(r.Context(), me.ID, req.StorageKey, "uploads", "private", "avatars")
	if err != nil {
		switch {
		case errors.Is(err, errInvalidStorageKey), errors.Is(err, errUnsupportedMedia):
			writeError(w, http.StatusBadRequest, "invalid media")
		case errors.Is(err, errObjectMissing):
			writeError(w, http.StatusNotFound, "object not found")
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	// Private media has no permanent public URL.
	publicURL := ""
	if storageClassForKey(confirmed.StorageKey) == storage.ClassPublic {
		publicURL = mediaURL(s.cfg.MediaPublicBaseURL, confirmed.StorageKey)
	}

	writeJSON(w, http.StatusOK, confirmUploadResponse{
		StorageKey: confirmed.StorageKey,
		Type:       confirmed.Type,
		MimeType:   confirmed.MimeType,
		SizeBytes:  confirmed.SizeBytes,
		PublicURL:  publicURL,
	})
}

// allowedUploadExtension validates type/mime/size against the size policy of
// the upload purpose and returns the safe extension.
func allowedUploadExtension(purpose, mediaType, mimeType string, sizeBytes int64) (string, bool) {
	return allowedExtensionWithLimits(limitsForPurpose(purpose), mediaType, mimeType, sizeBytes)
}

// allowedExtension validates type/mime/size under the post/story policy.
func allowedExtension(mediaType, mimeType string, sizeBytes int64) (string, bool) {
	return allowedExtensionWithLimits(postMediaLimits, mediaType, mimeType, sizeBytes)
}

func allowedExtensionWithLimits(limits mediaLimits, mediaType, mimeType string, sizeBytes int64) (string, bool) {
	mimeType = normalizedMIME(mimeType)
	var table map[string]string
	switch mediaType {
	case media.TypeImage:
		table = imageMimeExt
	case media.TypeVideo:
		table = videoMimeExt
	case "file":
		table = fileMimeExt
	case "voice":
		table = voiceMimeExt
	default:
		return "", false
	}
	ext, ok := table[mimeType]
	max := limits.limit(mediaType)
	if !ok || max <= 0 || sizeBytes > max {
		return "", false
	}
	return ext, true
}

func normalizedMIME(raw string) string {
	parsed, _, err := mime.ParseMediaType(strings.TrimSpace(raw))
	if err == nil {
		return strings.ToLower(parsed)
	}
	return strings.ToLower(strings.TrimSpace(strings.SplitN(raw, ";", 2)[0]))
}

// newUUIDv4 generates a random RFC 4122 v4 UUID string.
func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
