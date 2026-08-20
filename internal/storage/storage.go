// Package storage abstracts S3-compatible object storage. Business logic
// depends on ObjectStore only, so MinIO can be swapped for any external
// S3-compatible service via configuration.
package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ObjectStore is the platform's blob interface.
type ObjectStore interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// S3Store implements ObjectStore on any S3-compatible endpoint.
type S3Store struct {
	client *minio.Client
	bucket string
}

// Config for the object store.
type Config struct {
	Endpoint  string // e.g. http://localhost:9000
	AccessKey string
	SecretKey string
	Region    string
	Bucket    string
}

// NewS3Store connects and ensures the bucket exists.
func NewS3Store(ctx context.Context, cfg Config) (*S3Store, error) {
	u, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid S3 endpoint: %w", err)
	}
	client, err := minio.New(u.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: strings.EqualFold(u.Scheme, "https"),
		Region: cfg.Region,
	})
	if err != nil {
		return nil, err
	}
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("bucket check: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
			return nil, fmt.Errorf("bucket create: %w", err)
		}
	}
	return &S3Store{client: client, bucket: cfg.Bucket}, nil
}

func (s *S3Store) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	// GetObject is lazy; surface missing objects now.
	if _, err := obj.Stat(); err != nil {
		obj.Close()
		return nil, err
	}
	return obj, nil
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}
