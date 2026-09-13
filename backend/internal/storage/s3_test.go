package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// fakeS3 is a test double for the s3Client subset.
type fakeS3 struct {
	headOut *s3.HeadObjectOutput
	headErr error
}

func (f fakeS3) DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	return &s3.DeleteObjectOutput{}, nil
}

func (f fakeS3) HeadObject(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	return f.headOut, f.headErr
}

func newTestRepo(c s3Client) *S3Repository {
	return &S3Repository{client: c, bucket: "test-bucket"}
}

func TestHeadObjectSuccess(t *testing.T) {
	repo := newTestRepo(fakeS3{headOut: &s3.HeadObjectOutput{
		ContentType:   aws.String("image/png"),
		ContentLength: aws.Int64(1234),
	}})

	info, err := repo.HeadObject(context.Background(), "users/u/uploads/x.png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ContentType != "image/png" || info.SizeBytes != 1234 {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestHeadObjectNotFound(t *testing.T) {
	repo := newTestRepo(fakeS3{headErr: &types.NotFound{}})

	_, err := repo.HeadObject(context.Background(), "missing")
	if !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("expected ErrObjectNotFound, got %v", err)
	}
}

func TestHeadObjectStorageError(t *testing.T) {
	boom := errors.New("boom")
	repo := newTestRepo(fakeS3{headErr: boom})

	_, err := repo.HeadObject(context.Background(), "x")
	if err == nil || errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("expected a non-notfound error, got %v", err)
	}
}
