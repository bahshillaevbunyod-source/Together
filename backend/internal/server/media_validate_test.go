package server

import (
	"context"
	"errors"
	"testing"

	"together/backend/internal/media"
	"together/backend/internal/storage"
)

func validateServer(sr *fakeStorageRepo) *Server {
	return &Server{storage: sr}
}

func TestValidateUploadedObjectImage(t *testing.T) {
	sr := &fakeStorageRepo{headInfo: &storage.ObjectInfo{ContentType: "image/png", SizeBytes: 1000}}
	got, err := validateServer(sr).validateUploadedObject(context.Background(), "me-id", "users/me-id/uploads/x.png", "uploads")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Type != media.TypeImage || got.MimeType != "image/png" || got.SizeBytes != 1000 {
		t.Fatalf("unexpected result: %+v", got)
	}
	if got.StorageKey != "users/me-id/uploads/x.png" {
		t.Fatalf("unexpected key: %q", got.StorageKey)
	}
}

func TestValidateUploadedObjectFile(t *testing.T) {
	sr := &fakeStorageRepo{headInfo: &storage.ObjectInfo{ContentType: "application/pdf", SizeBytes: 2048}}
	got, err := validateServer(sr).validateUploadedObject(context.Background(), "me-id", "users/me-id/private/report.pdf", "private")
	if err != nil || got.Type != "file" {
		t.Fatalf("expected validated file, got=%+v err=%v", got, err)
	}
}

func TestAttachmentUploadPolicy(t *testing.T) {
	if ext, ok := allowedExtension("image", "image/png", 100); !ok || ext != "png" {
		t.Fatal("png image should be accepted")
	}
	if ext, ok := allowedExtension("file", "application/pdf", 100); !ok || ext != "pdf" {
		t.Fatal("pdf file should be accepted")
	}
	if _, ok := allowedExtension("file", "application/x-msdownload", 100); ok {
		t.Fatal("executable MIME must be rejected")
	}
	if _, ok := allowedExtension("file", "application/pdf", maxFileBytes+1); ok {
		t.Fatal("oversized file must be rejected")
	}
	if ext, ok := allowedExtension("voice", "audio/webm", 100); !ok || ext != "webm" {
		t.Fatal("webm voice should be accepted")
	}
	if ext, ok := allowedExtension("voice", "audio/ogg", 100); !ok || ext != "ogg" {
		t.Fatal("ogg voice should be accepted")
	}
	if _, ok := allowedExtension("voice", "audio/wav", 100); ok {
		t.Fatal("unsupported audio MIME must be rejected")
	}
}

func TestValidateUploadedObjectVoice(t *testing.T) {
	sr := &fakeStorageRepo{headInfo: &storage.ObjectInfo{ContentType: "audio/webm", SizeBytes: 2048}}
	got, err := validateServer(sr).validateUploadedObject(context.Background(), "me-id", "users/me-id/private/voice.webm", "private")
	if err != nil || got.Type != "voice" || got.MimeType != "audio/webm" {
		t.Fatalf("expected validated voice, got=%+v err=%v", got, err)
	}
}

func TestValidateUploadedObjectVideo(t *testing.T) {
	sr := &fakeStorageRepo{headInfo: &storage.ObjectInfo{ContentType: "video/mp4", SizeBytes: 1 << 20}}
	got, err := validateServer(sr).validateUploadedObject(context.Background(), "me-id", "users/me-id/uploads/v.mp4", "uploads")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Type != media.TypeVideo {
		t.Fatalf("expected video, got %q", got.Type)
	}
}

func TestValidateUploadedObjectAvatarDir(t *testing.T) {
	sr := &fakeStorageRepo{headInfo: &storage.ObjectInfo{ContentType: "image/png", SizeBytes: 1000}}
	got, err := validateServer(sr).validateUploadedObject(
		context.Background(), "me-id", "users/me-id/avatars/a.png", "uploads", "avatars")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.StorageKey != "users/me-id/avatars/a.png" {
		t.Fatalf("unexpected key: %q", got.StorageKey)
	}
}

func TestValidateUploadedObjectDirNotAllowed(t *testing.T) {
	sr := &fakeStorageRepo{headInfo: &storage.ObjectInfo{ContentType: "image/png", SizeBytes: 1000}}
	// avatar key rejected when only uploads is allowed (post attachment case)
	if _, err := validateServer(sr).validateUploadedObject(
		context.Background(), "me-id", "users/me-id/avatars/a.png", "uploads"); !errors.Is(err, errInvalidStorageKey) {
		t.Fatalf("avatar key under uploads-only: expected errInvalidStorageKey, got %v", err)
	}
	// uploads key rejected when only avatars is allowed
	if _, err := validateServer(sr).validateUploadedObject(
		context.Background(), "me-id", "users/me-id/uploads/x.png", "avatars"); !errors.Is(err, errInvalidStorageKey) {
		t.Fatalf("uploads key under avatars-only: expected errInvalidStorageKey, got %v", err)
	}
	// unknown dir rejected
	if _, err := validateServer(sr).validateUploadedObject(
		context.Background(), "me-id", "users/me-id/secret/x.png", "uploads", "avatars"); !errors.Is(err, errInvalidStorageKey) {
		t.Fatalf("unknown dir: expected errInvalidStorageKey, got %v", err)
	}
}

func TestValidateUploadedObjectForeignPrefix(t *testing.T) {
	sr := &fakeStorageRepo{headInfo: &storage.ObjectInfo{ContentType: "image/png", SizeBytes: 1000}}
	_, err := validateServer(sr).validateUploadedObject(context.Background(), "me-id", "users/other-id/uploads/x.png", "uploads")
	if !errors.Is(err, errInvalidStorageKey) {
		t.Fatalf("expected errInvalidStorageKey, got %v", err)
	}
}

func TestValidateUploadedObjectTraversal(t *testing.T) {
	sr := &fakeStorageRepo{headInfo: &storage.ObjectInfo{ContentType: "image/png", SizeBytes: 1000}}
	for _, key := range []string{
		"users/me-id/uploads/../../etc/passwd",
		"users/me-id/uploads/..",
		`users/me-id/uploads/a\b`,
		"users/me-id/uploads/nested/x.png",
	} {
		if _, err := validateServer(sr).validateUploadedObject(context.Background(), "me-id", key, "uploads"); !errors.Is(err, errInvalidStorageKey) {
			t.Fatalf("key %q: expected errInvalidStorageKey, got %v", key, err)
		}
	}
}

func TestValidateUploadedObjectMissing(t *testing.T) {
	sr := &fakeStorageRepo{headErr: storage.ErrObjectNotFound}
	_, err := validateServer(sr).validateUploadedObject(context.Background(), "me-id", "users/me-id/uploads/x.png", "uploads")
	if !errors.Is(err, errObjectMissing) {
		t.Fatalf("expected errObjectMissing, got %v", err)
	}
}

func TestValidateUploadedObjectInvalidMime(t *testing.T) {
	sr := &fakeStorageRepo{headInfo: &storage.ObjectInfo{ContentType: "image/gif", SizeBytes: 1000}}
	_, err := validateServer(sr).validateUploadedObject(context.Background(), "me-id", "users/me-id/uploads/x.gif", "uploads")
	if !errors.Is(err, errUnsupportedMedia) {
		t.Fatalf("expected errUnsupportedMedia, got %v", err)
	}
}

func TestValidateUploadedObjectSizeOverLimit(t *testing.T) {
	sr := &fakeStorageRepo{headInfo: &storage.ObjectInfo{ContentType: "image/png", SizeBytes: 16 << 20}}
	_, err := validateServer(sr).validateUploadedObject(context.Background(), "me-id", "users/me-id/uploads/x.png", "uploads")
	if !errors.Is(err, errUnsupportedMedia) {
		t.Fatalf("expected errUnsupportedMedia, got %v", err)
	}
}
