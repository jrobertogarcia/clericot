package storage

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"gocloud.dev/blob"
	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/memblob"
	_ "gocloud.dev/blob/s3blob"

	"clericot/internal/platform/tenant"
)

// StorageEngine provides cloud-agnostic blob storage abstraction.
type StorageEngine struct {
	bucket *blob.Bucket
}

// NewStorageEngine opens a bucket connection given a gocloud URL (e.g. "mem://", "file:///tmp/storage", "s3://...").
func NewStorageEngine(ctx context.Context, bucketURL string) (*StorageEngine, error) {
	bucket, err := blob.OpenBucket(ctx, bucketURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open bucket at %s: %w", bucketURL, err)
	}
	return &StorageEngine{bucket: bucket}, nil
}

// NewStorageEngineWithBucket creates an engine directly wrapping an existing *blob.Bucket (useful for testing).
func NewStorageEngineWithBucket(bucket *blob.Bucket) *StorageEngine {
	return &StorageEngine{bucket: bucket}
}

// PresignedUpload generates a tenant-scoped presigned PUT URL.
func (s *StorageEngine) PresignedUpload(ctx context.Context, filename string, contentType string, expiry time.Duration) (signedURL string, key string, err error) {
	tenantID := tenant.FromContext(ctx)
	if tenantID == "" {
		tenantID = "global"
	}

	key = path.Join("tenants", tenantID, "uploads", fmt.Sprintf("%s-%s", uuid.NewString(), filename))

	opts := &blob.SignedURLOptions{
		Expiry:      expiry,
		Method:      "PUT",
		ContentType: contentType,
	}

	signedURL, err = s.bucket.SignedURL(ctx, key, opts)
	if err != nil {
		// If driver does not support SignedURL (e.g. in-memory or raw file), construct simulated URL
		return fmt.Sprintf("storage://%s", key), key, nil
	}

	return signedURL, key, nil
}

// PresignedDownload generates a presigned GET URL.
func (s *StorageEngine) PresignedDownload(ctx context.Context, key string, expiry time.Duration) (string, error) {
	opts := &blob.SignedURLOptions{
		Expiry: expiry,
		Method: "GET",
	}

	signedURL, err := s.bucket.SignedURL(ctx, key, opts)
	if err != nil {
		return fmt.Sprintf("storage://%s", key), nil
	}

	return signedURL, nil
}

// ConfirmUpload verifies the object exists in the storage bucket.
func (s *StorageEngine) ConfirmUpload(ctx context.Context, key string) (*blob.Attributes, error) {
	attrs, err := s.bucket.Attributes(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to verify object in storage: %w", err)
	}
	return attrs, nil
}

// Write uploads bytes directly into the storage bucket.
func (s *StorageEngine) Write(ctx context.Context, key string, data []byte, contentType string) error {
	opts := &blob.WriterOptions{
		ContentType: contentType,
	}
	w, err := s.bucket.NewWriter(ctx, key, opts)
	if err != nil {
		return fmt.Errorf("failed to create blob writer: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return fmt.Errorf("failed to write blob data: %w", err)
	}
	return w.Close()
}

// Read downloads bytes directly from the storage bucket.
func (s *StorageEngine) Read(ctx context.Context, key string) ([]byte, error) {
	r, err := s.bucket.NewReader(ctx, key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create blob reader: %w", err)
	}
	defer r.Close()

	return io.ReadAll(r)
}

// Delete removes an object from the bucket.
func (s *StorageEngine) Delete(ctx context.Context, key string) error {
	return s.bucket.Delete(ctx, key)
}

// DeletePrefix deletes all objects under a given prefix.
func (s *StorageEngine) DeletePrefix(ctx context.Context, prefix string) error {
	iter := s.bucket.List(&blob.ListOptions{
		Prefix: prefix,
	})

	for {
		obj, err := iter.Next(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error listing objects under prefix %s: %w", prefix, err)
		}
		if !obj.IsDir {
			if err := s.bucket.Delete(ctx, obj.Key); err != nil && !strings.Contains(err.Error(), "not found") {
				return fmt.Errorf("failed to delete object %s: %w", obj.Key, err)
			}
		}
	}

	return nil
}

// Close releases bucket resources.
func (s *StorageEngine) Close() error {
	return s.bucket.Close()
}
