package minio

import (
	"context"
	"strings"
	"testing"
	"time"

	miniosdk "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Config struct {
	Endpoint, AccessKey, SecretKey string
	Bucket, Region                 string
	Secure                         bool
}

type OwnedObjectCleaner struct {
	t      testing.TB
	client *miniosdk.Client
	bucket string
	keys   map[string]struct{}
}

func NewOwnedObjectCleaner(t testing.TB, config Config) *OwnedObjectCleaner {
	t.Helper()
	config.Endpoint = strings.TrimSpace(config.Endpoint)
	config.AccessKey = strings.TrimSpace(config.AccessKey)
	config.SecretKey = strings.TrimSpace(config.SecretKey)
	config.Bucket = strings.TrimSpace(config.Bucket)
	if config.Endpoint == "" || config.AccessKey == "" || config.SecretKey == "" || config.Bucket == "" {
		t.Fatal("MinIO owned-object cleanup requires endpoint, credentials, and bucket")
	}
	client, err := miniosdk.New(config.Endpoint, &miniosdk.Options{
		Creds:  credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
		Secure: config.Secure,
		Region: strings.TrimSpace(config.Region),
	})
	if err != nil {
		t.Fatalf("open MinIO owned-object cleanup client: %v", err)
	}
	cleaner := &OwnedObjectCleaner{t: t, client: client, bucket: config.Bucket, keys: make(map[string]struct{})}
	t.Cleanup(cleaner.removeTrackedObjects)
	return cleaner
}

func (cleaner *OwnedObjectCleaner) Track(objectKey string) {
	cleaner.t.Helper()
	objectKey = strings.TrimSpace(objectKey)
	if objectKey == "" || strings.HasPrefix(objectKey, "/") || strings.HasSuffix(objectKey, "/") || strings.Contains(objectKey, "..") {
		cleaner.t.Fatalf("refuse to track invalid MinIO object key %q", objectKey)
	}
	cleaner.keys[objectKey] = struct{}{}
}

func (cleaner *OwnedObjectCleaner) removeTrackedObjects() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for objectKey := range cleaner.keys {
		if err := cleaner.client.RemoveObject(ctx, cleaner.bucket, objectKey, miniosdk.RemoveObjectOptions{}); err != nil {
			cleaner.t.Errorf("remove owned MinIO object %q: %v", objectKey, err)
			continue
		}
		if _, err := cleaner.client.StatObject(ctx, cleaner.bucket, objectKey, miniosdk.StatObjectOptions{}); err == nil {
			cleaner.t.Errorf("owned MinIO object %q still exists after cleanup", objectKey)
		} else if response := miniosdk.ToErrorResponse(err); response.StatusCode != 404 && response.Code != "NoSuchKey" && response.Code != "NoSuchObject" {
			cleaner.t.Errorf("verify owned MinIO object %q cleanup: %v", objectKey, err)
		}
	}
}
