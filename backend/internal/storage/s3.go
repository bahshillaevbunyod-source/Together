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

// bucketClient bundles the SDK client, presigner and bucket name for one
// storage class (one R2 bucket, with its own credentials).
type bucketClient struct {
	client  s3Client
	presign *s3.PresignClient
	bucket  string
}

// S3Repository is the production Repository backed by S3-compatible storage
// (Cloudflare R2). It holds one bucketClient per configured storage class.
type S3Repository struct {
	buckets map[Class]*bucketClient
}

// Compile-time assurance that the implementation satisfies Repository.
var _ Repository = (*S3Repository)(nil)

// NewS3Repository builds the storage repository from config. The public bucket
// (STORAGE_BUCKET + STORAGE_* credentials) is always wired. The private bucket
// (STORAGE_PRIVATE_BUCKET + STORAGE_PRIVATE_* credentials) is wired only when
// configured; private operations otherwise return ErrClassNotConfigured.
// Credentials are held by the SDK; nothing here logs them.
func NewS3Repository(cfg config.Config) *S3Repository {
	buckets := map[Class]*bucketClient{}

	if cfg.StorageBucket != "" {
		buckets[ClassPublic] = newBucketClient(
			cfg.StorageEndpoint, cfg.StorageRegion,
			cfg.StorageAccessKeyID, cfg.StorageSecretAccessKey, cfg.StorageBucket,
		)
	}
	if cfg.StoragePrivateBucket != "" &&
		cfg.StoragePrivateAccessKeyID != "" && cfg.StoragePrivateSecretAccessKey != "" {
		buckets[ClassPrivate] = newBucketClient(
			cfg.StorageEndpoint, cfg.StorageRegion,
			cfg.StoragePrivateAccessKeyID, cfg.StoragePrivateSecretAccessKey, cfg.StoragePrivateBucket,
		)
	}

	return &S3Repository{buckets: buckets}
}

func newBucketClient(endpoint, region, accessKeyID, secret, bucket string) *bucketClient {
	client := s3.New(s3.Options{
		Region:       region,
		Credentials:  credentials.NewStaticCredentialsProvider(accessKeyID, secret, ""),
		BaseEndpoint: aws.String(endpoint),
		UsePathStyle: true, // required by most S3-compatible endpoints (R2, MinIO)
	})
	return &bucketClient{client: client, presign: s3.NewPresignClient(client), bucket: bucket}
}

func (r *S3Repository) bucketFor(class Class) (*bucketClient, error) {
	if bc, ok := r.buckets[class]; ok && bc != nil {
		return bc, nil
	}
	return nil, ErrClassNotConfigured
}

// Configured reports whether the given storage class has a bucket wired up.
func (r *S3Repository) Configured(class Class) bool {
	bc, ok := r.buckets[class]
	return ok && bc != nil
}

// CreateUploadURL returns a presigned PUT URL for a direct client upload,
// pinned to the given mimeType and sizeBytes and valid for 10 minutes.
func (r *S3Repository) CreateUploadURL(ctx context.Context, class Class, key, mimeType string, sizeBytes int64) (*PresignedUpload, error) {
	bc, err := r.bucketFor(class)
	if err != nil {
		return nil, err
	}
	req, err := bc.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(bc.bucket),
		Key:           aws.String(key),
		ContentType:   aws.String(mimeType),
		ContentLength: aws.Int64(sizeBytes),
	}, s3.WithPresignExpires(uploadURLExpiry))
	if err != nil {
		return nil, err
	}
	return &PresignedUpload{UploadURL: req.URL, ExpiresAt: time.Now().Add(uploadURLExpiry)}, nil
}

// PresignGetURL returns a short-lived presigned GET URL for reading an object.
func (r *S3Repository) PresignGetURL(ctx context.Context, class Class, key string, expiry time.Duration) (string, error) {
	bc, err := r.bucketFor(class)
	if err != nil {
		return "", err
	}
	req, err := bc.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bc.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// DeleteObject removes the object at key in the given class.
func (r *S3Repository) DeleteObject(ctx context.Context, class Class, key string) error {
	bc, err := r.bucketFor(class)
	if err != nil {
		return err
	}
	_, err = bc.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bc.bucket),
		Key:    aws.String(key),
	})
	return err
}

// HeadObject returns metadata for the object at key, or ErrObjectNotFound.
func (r *S3Repository) HeadObject(ctx context.Context, class Class, key string) (*ObjectInfo, error) {
	bc, err := r.bucketFor(class)
	if err != nil {
		return nil, err
	}
	out, err := bc.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bc.bucket),
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
