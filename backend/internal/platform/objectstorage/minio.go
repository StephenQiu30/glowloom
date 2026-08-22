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

type Store struct {
	client *minio.Client
	bucket string
}

func New(ctx context.Context) (*Store, error) {
	endpoint := envOr("MINIO_ENDPOINT", defaultEndpoint)
	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")
	accessKey := envOr("MINIO_ACCESS_KEY", "lanverse")
	secretKey := envOr("MINIO_SECRET_KEY", "lanverse-development-only")
	secure := strings.EqualFold(os.Getenv("MINIO_SECURE"), "true")
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: secure})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	store := &Store{client: client, bucket: envOr("MINIO_BUCKET", defaultBucket)}
	exists, err := client.BucketExists(ctx, store.bucket)
	if err != nil {
		return nil, fmt.Errorf("check minio bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, store.bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("create minio bucket: %w", err)
		}
	}
	return store, nil
}

func (s *Store) Put(ctx context.Context, key string, content []byte, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(content), int64(len(content)), minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("put object %s: %w", key, err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %s: %w", key, err)
	}
	defer object.Close()
	content, err := io.ReadAll(object)
	if err != nil {
		return nil, fmt.Errorf("read object %s: %w", key, err)
	}
	return content, nil
}

func (s *Store) Bucket() string { return s.bucket }

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
