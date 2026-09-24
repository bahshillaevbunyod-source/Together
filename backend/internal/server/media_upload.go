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

const (
	maxUploadBodyBytes = 1 << 20 // 1 MiB (metadata only)
	maxImageBytes      = 15 << 20
	maxVideoBytes      = 200 << 20
	maxFileBytes       = 25 << 20
)

// allowed mime -> safe file extension, per media type.
var imageMimeExt = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
}

var videoMimeExt = map[string]string{
	"video/mp4":  "mp4",
	"video/webm": "webm",
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
	// Purpose selects the storage namespace: "" or "post" -> post media,
	// "avatar" -> avatars/ (profile photos). Kept optional for backward
	// compatibility (existing post clients send no purpose).
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
	case "", "post":
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
	ext, ok := allowedExtension(req.Type, req.MimeType, req.SizeBytes)
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

// allowedExtension validates type/mime/size and returns the safe extension.
func allowedExtension(mediaType, mimeType string, sizeBytes int64) (string, bool) {
	mimeType = normalizedMIME(mimeType)
	switch mediaType {
	case media.TypeImage:
		ext, ok := imageMimeExt[mimeType]
		if !ok || sizeBytes > maxImageBytes {
			return "", false
		}
		return ext, true
	case media.TypeVideo:
		ext, ok := videoMimeExt[mimeType]
		if !ok || sizeBytes > maxVideoBytes {
			return "", false
		}
		return ext, true
	case "file":
		ext, ok := fileMimeExt[mimeType]
		if !ok || sizeBytes > maxFileBytes {
			return "", false
		}
		return ext, true
	case "voice":
		ext, ok := voiceMimeExt[mimeType]
		if !ok || sizeBytes > maxFileBytes {
			return "", false
		}
		return ext, true
	default:
		return "", false
	}
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
