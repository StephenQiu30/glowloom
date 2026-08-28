package platform_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstore"
)

func TestPrivateObjectEnsureIsIdempotentAndRejectsContentDrift(t *testing.T) {
	endpoint := os.Getenv("LANVERSE_TEST_MINIO_ENDPOINT")
	accessKey := os.Getenv("LANVERSE_TEST_MINIO_ACCESS_KEY")
	secretKey := os.Getenv("LANVERSE_TEST_MINIO_SECRET_KEY")
	bucket := os.Getenv("LANVERSE_TEST_MINIO_BUCKET")
	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		t.Skip("set MinIO test variables to run private object staging integration")
	}
	ctx := context.Background()
	client, err := objectstore.Open(objectstore.Config{
		Endpoint: endpoint, PublicEndpoint: endpoint, AccessKey: accessKey, SecretKey: secretKey,
		Bucket: bucket, Region: "us-east-1",
	})
	if err != nil {
		t.Fatalf("open private object staging client: %v", err)
	}
	if err = client.EnsureBucket(ctx); err != nil {
		t.Fatalf("ensure private object staging bucket: %v", err)
	}
	cleanup, err := minio.New(endpoint, &minio.Options{
		Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Region: "us-east-1",
	})
	if err != nil {
		t.Fatalf("open private object cleanup client: %v", err)
	}
	objectKey := "staging/" + uuid.NewString() + "/" + uuid.NewString() + "/" + uuid.NewString() + ".png"
	t.Cleanup(func() {
		if cleanupErr := cleanup.RemoveObject(ctx, bucket, objectKey, minio.RemoveObjectOptions{}); cleanupErr != nil {
			t.Errorf("remove test-owned private object: %v", cleanupErr)
		}
	})
	contents := []byte("stable-private-object")
	contentsHash := privateObjectSHA256(contents)
	if err = client.EnsurePrivateObject(ctx, objectKey, contents, "image/png", contentsHash); err != nil {
		t.Fatalf("write private object: %v", err)
	}
	if err = client.EnsurePrivateObject(ctx, objectKey, contents, "image/png", contentsHash); err != nil {
		t.Fatalf("replay private object ensure: %v", err)
	}
	if err = client.EnsurePrivateObject(ctx, objectKey, contents, "image/jpeg", contentsHash); !errors.Is(err, objectstore.ErrInvalidObjectDeclaration) {
		t.Fatalf("same private object key accepted media type drift: %T %v", err, err)
	}
	drifted := []byte("drifted-private-value")
	if err = client.EnsurePrivateObject(ctx, objectKey, drifted, "image/png", privateObjectSHA256(drifted)); err == nil || (!errors.Is(err, objectstore.ErrObjectSizeMismatch) &&
		!errors.Is(err, objectstore.ErrObjectChecksumMismatch)) {
		t.Fatalf("same private object key accepted content drift: %T %v", err, err)
	}
	stored, err := client.ReadVerified(ctx, objectKey, int64(len(contents)), contentsHash, int64(len(contents)))
	if err != nil || string(stored) != string(contents) {
		t.Fatalf("content drift changed private object: contents=%q err=%v", stored, err)
	}

	concurrentKey := "staging/" + uuid.NewString() + "/" + uuid.NewString() + "/" + uuid.NewString() + ".png"
	t.Cleanup(func() {
		if cleanupErr := cleanup.RemoveObject(ctx, bucket, concurrentKey, minio.RemoveObjectOptions{}); cleanupErr != nil {
			t.Errorf("remove concurrent test-owned private object: %v", cleanupErr)
		}
	})
	concurrentValues := [][]byte{[]byte("concurrent-private-a"), []byte("concurrent-private-b")}
	start := make(chan struct{})
	results := make(chan error, len(concurrentValues))
	var writers sync.WaitGroup
	for _, value := range concurrentValues {
		writers.Add(1)
		go func() {
			defer writers.Done()
			<-start
			results <- client.EnsurePrivateObject(
				ctx, concurrentKey, value, "image/png", privateObjectSHA256(value),
			)
		}()
	}
	close(start)
	writers.Wait()
	close(results)
	successes, conflicts := 0, 0
	for writeErr := range results {
		if writeErr == nil {
			successes++
			continue
		}
		if errors.Is(writeErr, objectstore.ErrObjectSizeMismatch) ||
			errors.Is(writeErr, objectstore.ErrObjectChecksumMismatch) {
			conflicts++
			continue
		}
		t.Fatalf("concurrent private object ensure returned unexpected error: %T %v", writeErr, writeErr)
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent private object writes = %d success/%d conflict, want 1/1", successes, conflicts)
	}
}

func privateObjectSHA256(contents []byte) string {
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}
