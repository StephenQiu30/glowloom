package objectstorage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const defaultEndpoint = "127.0.0.1:9000"
const defaultBucket = "lanverse-media"

type MinIOObjectStore struct {
	client         *minio.Client
	storageProfile string
	bucket         string
}

type ObjectVersion struct {
	StorageProfile string
	Bucket         string
	Key            string
	VersionID      string
	ETag           string
	Size           int64
}

type Port interface {
	PutVersioned(context.Context, string, []byte, string) (ObjectVersion, error)
	GetVersioned(context.Context, string, string) ([]byte, error)
	DeleteVersion(context.Context, string, string) error
}

func NewMinIOObjectStore(ctx context.Context) (*MinIOObjectStore, error) {
	endpoint := envOr("MINIO_ENDPOINT", defaultEndpoint)
	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")
	accessKey := envOr("MINIO_ACCESS_KEY", "lanverse")
	secretKey := envOr("MINIO_SECRET_KEY", "lanverse-development-only")
	secure := strings.EqualFold(os.Getenv("MINIO_SECURE"), "true")
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: secure})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	store := &MinIOObjectStore{client: client, storageProfile: envOr("MINIO_STORAGE_PROFILE", "primary"), bucket: envOr("MINIO_BUCKET", defaultBucket)}
	exists, err := client.BucketExists(ctx, store.bucket)
	if err != nil {
		return nil, fmt.Errorf("check minio bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, store.bucket, minio.MakeBucketOptions{}); err != nil {
			// API and workers are expected to start concurrently. Another role may
			// create the same bucket after BucketExists and before MakeBucket; only
			// the explicit same-owner result is safe to treat as success.
			if minio.ToErrorResponse(err).Code != minio.BucketAlreadyOwnedByYou {
				return nil, fmt.Errorf("create minio bucket: %w", err)
			}
		}
	}
	// Every object reference records an exact version. Enabling versioning is
	// part of the current storage contract and is safe on an already-created
	// bucket.
	if err := client.SetBucketVersioning(ctx, store.bucket, minio.BucketVersioningConfiguration{Status: "Enabled"}); err != nil {
		return nil, fmt.Errorf("enable minio versioning: %w", err)
	}
	return store, nil
}

func (s *MinIOObjectStore) PutVersioned(ctx context.Context, key string, content []byte, contentType string) (ObjectVersion, error) {
	info, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(content), int64(len(content)), minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return ObjectVersion{}, fmt.Errorf("put versioned object: %w", err)
	}
	return ObjectVersion{StorageProfile: s.storageProfile, Bucket: s.bucket, Key: key, VersionID: info.VersionID, ETag: info.ETag, Size: info.Size}, nil
}

func (s *MinIOObjectStore) GetVersioned(ctx context.Context, key, versionID string) ([]byte, error) {
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{VersionID: versionID})
	if err != nil {
		return nil, fmt.Errorf("get versioned object: %w", err)
	}
	defer object.Close()
	content, err := io.ReadAll(object)
	if err != nil {
		return nil, fmt.Errorf("read versioned object: %w", err)
	}
	return content, nil
}

func (s *MinIOObjectStore) DeleteVersion(ctx context.Context, key, versionID string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{VersionID: versionID}); err != nil {
		return fmt.Errorf("delete versioned object: %w", err)
	}
	return nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
