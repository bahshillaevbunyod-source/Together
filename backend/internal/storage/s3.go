package storage

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"together/backend/internal/config"
)

// uploadURLExpiry is how long a presigned upload URL stays valid.
const uploadURLExpiry = 10 * time.Minute

// s3Client is the subset of *s3.Client this repository uses (allows faking in
// tests). *s3.Client satisfies it.
type s3Client interface {
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
}

// S3Repository is the production Repository backed by an S3-compatible service
// (AWS S3 / Cloudflare R2) via AWS SDK for Go v2.
type S3Repository struct {
	client  s3Client
	presign *s3.PresignClient
	bucket  string
}

// Compile-time assurance that the implementation satisfies Repository.
var _ Repository = (*S3Repository)(nil)

// NewS3Repository builds an S3-compatible storage repository from config.
// Credentials are held by the SDK; nothing here logs them.
func NewS3Repository(cfg config.Config) *S3Repository {
	client := s3.New(s3.Options{
		Region: cfg.StorageRegion,
		Credentials: credentials.NewStaticCredentialsProvider(
			cfg.StorageAccessKeyID, cfg.StorageSecretAccessKey, "",
		),
		BaseEndpoint: aws.String(cfg.StorageEndpoint),
		UsePathStyle: true, // required by most S3-compatible endpoints (R2, MinIO)
	})

	return &S3Repository{
		client:  client,
		presign: s3.NewPresignClient(client),
		bucket:  cfg.StorageBucket,
	}
}

// CreateUploadURL returns a presigned PUT URL for a direct client upload,
// pinned to the given mimeType and sizeBytes and valid for 10 minutes.
func (r *S3Repository) CreateUploadURL(ctx context.Context, key, mimeType string, sizeBytes int64) (*PresignedUpload, error) {
	req, err := r.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(r.bucket),
		Key:           aws.String(key),
		ContentType:   aws.String(mimeType),
		ContentLength: aws.Int64(sizeBytes),
	}, s3.WithPresignExpires(uploadURLExpiry))
	if err != nil {
		return nil, err
	}

	return &PresignedUpload{
		UploadURL: req.URL,
		ExpiresAt: time.Now().Add(uploadURLExpiry),
	}, nil
}

// DeleteObject removes the object at key.
func (r *S3Repository) DeleteObject(ctx context.Context, key string) error {
	_, err := r.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
	})
	return err
}

// HeadObject returns metadata for the object at key, or ErrObjectNotFound.
func (r *S3Repository) HeadObject(ctx context.Context, key string) (*ObjectInfo, error) {
	out, err := r.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return nil, ErrObjectNotFound
		}
		return nil, err
	}

	info := &ObjectInfo{}
	if out.ContentType != nil {
		info.ContentType = *out.ContentType
	}
	if out.ContentLength != nil {
		info.SizeBytes = *out.ContentLength
	}
	return info, nil
}
