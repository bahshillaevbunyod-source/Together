package server

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"together/backend/internal/media"
)

const (
	maxUploadBodyBytes = 1 << 20 // 1 MiB (metadata only)
	maxImageBytes      = 15 << 20
	maxVideoBytes      = 200 << 20
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

type uploadURLRequest struct {
	Type      string `json:"type"`
	MimeType  string `json:"mimeType"`
	SizeBytes int64  `json:"sizeBytes"`
	// Purpose selects the storage namespace: "" or "post" -> uploads/ (post
	// media), "avatar" -> avatars/ (profile photos). Kept optional for backward
	// compatibility (existing post clients send no purpose).
	Purpose string `json:"purpose"`
}

// uploadDirForPurpose maps a client purpose to the storage subdirectory.
func uploadDirForPurpose(purpose string) (string, bool) {
	switch purpose {
	case "", "post":
		return "uploads", true
	case "avatar":
		return "avatars", true
	default:
		return "", false
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

	dir, ok := uploadDirForPurpose(req.Purpose)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid purpose")
		return
	}

	id, err := newUUIDv4()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Key is fully server-generated: user id from auth context, namespace dir,
	// uuid, safe ext.
	key := fmt.Sprintf("users/%s/%s/%s.%s", me.ID, dir, id, ext)

	upload, err := s.storage.CreateUploadURL(r.Context(), key, req.MimeType, req.SizeBytes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, uploadURLResponse{
		UploadURL:  upload.UploadURL,
		StorageKey: key,
		PublicURL:  mediaURL(s.cfg.MediaPublicBaseURL, key),
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

	// Confirm accepts either namespace (post uploads or avatars); the key's dir
	// determines where it lives.
	confirmed, err := s.validateUploadedObject(r.Context(), me.ID, req.StorageKey, "uploads", "avatars")
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

	writeJSON(w, http.StatusOK, confirmUploadResponse{
		StorageKey: confirmed.StorageKey,
		Type:       confirmed.Type,
		MimeType:   confirmed.MimeType,
		SizeBytes:  confirmed.SizeBytes,
		PublicURL:  mediaURL(s.cfg.MediaPublicBaseURL, confirmed.StorageKey),
	})
}

// allowedExtension validates type/mime/size and returns the safe extension.
func allowedExtension(mediaType, mimeType string, sizeBytes int64) (string, bool) {
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
	default:
		return "", false
	}
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
