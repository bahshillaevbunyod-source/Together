package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"together/backend/internal/storage"
)

func confirmUpload(srv *http.Server, body string, withCookie, withOrigin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/confirm", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if withCookie {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	}
	if withOrigin {
		req.Header.Set("Origin", testOrigin)
	}
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func TestConfirmUploadValid(t *testing.T) {
	sr := &fakeStorageRepo{headInfo: &storage.ObjectInfo{ContentType: "image/png", SizeBytes: 2048}}
	rec := confirmUpload(uploadServer(sr), `{"storageKey":"users/me-id/uploads/x.png"}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp confirmUploadResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Type != "image" || resp.MimeType != "image/png" || resp.SizeBytes != 2048 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.PublicURL != "https://cdn.example.com/media/users/me-id/uploads/x.png" {
		t.Fatalf("unexpected publicUrl: %q", resp.PublicURL)
	}
}

func TestConfirmUploadForeignKey(t *testing.T) {
	sr := &fakeStorageRepo{headInfo: &storage.ObjectInfo{ContentType: "image/png", SizeBytes: 2048}}
	rec := confirmUpload(uploadServer(sr), `{"storageKey":"users/other/uploads/x.png"}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestConfirmUploadMissingObject(t *testing.T) {
	sr := &fakeStorageRepo{headErr: storage.ErrObjectNotFound}
	rec := confirmUpload(uploadServer(sr), `{"storageKey":"users/me-id/uploads/x.png"}`, true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestConfirmUploadInvalidMedia(t *testing.T) {
	sr := &fakeStorageRepo{headInfo: &storage.ObjectInfo{ContentType: "image/gif", SizeBytes: 2048}}
	rec := confirmUpload(uploadServer(sr), `{"storageKey":"users/me-id/uploads/x.gif"}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestConfirmUploadStorageError(t *testing.T) {
	sr := &fakeStorageRepo{headErr: errForTest}
	rec := confirmUpload(uploadServer(sr), `{"storageKey":"users/me-id/uploads/x.png"}`, true, true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestConfirmUploadUnauthenticated(t *testing.T) {
	rec := confirmUpload(uploadServer(&fakeStorageRepo{}), `{"storageKey":"users/me-id/uploads/x.png"}`, false, true)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestConfirmUploadMissingOrigin(t *testing.T) {
	rec := confirmUpload(uploadServer(&fakeStorageRepo{}), `{"storageKey":"users/me-id/uploads/x.png"}`, true, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
