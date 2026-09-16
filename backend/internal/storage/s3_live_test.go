package storage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"together/backend/internal/config"
)

// TestPrivateBucketRoundTripLive verifies the configured PRIVATE credentials can
// presign+PUT+GET+DELETE against the private bucket through the real storage
// code. It is skipped unless the private storage env is present, so the normal
// (offline) test run is unaffected; run it with the backend .env loaded to check
// credentials. It never prints secrets.
func TestPrivateBucketRoundTripLive(t *testing.T) {
	if os.Getenv("STORAGE_PRIVATE_ACCESS_KEY_ID") == "" || os.Getenv("STORAGE_PRIVATE_BUCKET") == "" {
		t.Skip("private storage env not set; skipping live private-bucket check")
	}

	cfg := config.Config{
		StorageEndpoint:               os.Getenv("STORAGE_ENDPOINT"),
		StorageRegion:                 os.Getenv("STORAGE_REGION"),
		StorageBucket:                 os.Getenv("STORAGE_BUCKET"),
		StorageAccessKeyID:            os.Getenv("STORAGE_ACCESS_KEY_ID"),
		StorageSecretAccessKey:        os.Getenv("STORAGE_SECRET_ACCESS_KEY"),
		StoragePrivateBucket:          os.Getenv("STORAGE_PRIVATE_BUCKET"),
		StoragePrivateAccessKeyID:     os.Getenv("STORAGE_PRIVATE_ACCESS_KEY_ID"),
		StoragePrivateSecretAccessKey: os.Getenv("STORAGE_PRIVATE_SECRET_ACCESS_KEY"),
	}
	repo := NewS3Repository(cfg)
	if !repo.Configured(ClassPrivate) {
		t.Fatal("private storage class not configured from env")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	key := "diagnostics/live-test.txt"
	body := []byte("live-private-check")

	up, err := repo.CreateUploadURL(ctx, ClassPrivate, key, "text/plain", int64(len(body)))
	if err != nil {
		t.Fatalf("presign PUT failed: %v", err)
	}
	putReq, _ := http.NewRequestWithContext(ctx, http.MethodPut, up.UploadURL, bytes.NewReader(body))
	putReq.Header.Set("Content-Type", "text/plain")
	putRes, err := http.DefaultClient.Do(putReq)
	if err != nil {
		t.Fatalf("PUT request failed: %v", err)
	}
	putRes.Body.Close()
	if putRes.StatusCode/100 != 2 {
		t.Fatalf("PUT to private bucket returned %d (credentials cannot write)", putRes.StatusCode)
	}
	// Always attempt cleanup even if later steps fail.
	defer func() { _ = repo.DeleteObject(context.Background(), ClassPrivate, key) }()

	getURL, err := repo.PresignGetURL(ctx, ClassPrivate, key, 10*time.Minute)
	if err != nil {
		t.Fatalf("presign GET failed: %v", err)
	}
	getRes, err := http.Get(getURL)
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	got, _ := io.ReadAll(getRes.Body)
	getRes.Body.Close()
	if getRes.StatusCode/100 != 2 || !bytes.Equal(got, body) {
		t.Fatalf("presigned GET failed: status=%d match=%v", getRes.StatusCode, bytes.Equal(got, body))
	}
}
