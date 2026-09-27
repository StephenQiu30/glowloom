package objectstorage_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

func TestPingWithHostObjectStorage(t *testing.T) {
	if os.Getenv("LV_TEST_OBJECT_STORAGE") != "1" {
		t.Skip("set LV_TEST_OBJECT_STORAGE=1 and LV_ENV_FILE to verify the host object storage service")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load object storage configuration: %v", err)
	}
	conn, err := objectstorage.Open(cfg.ObjectStorageEndpoint, cfg.ObjectStorageBucket, cfg.ObjectStorageAccessKey, cfg.ObjectStorageSecretKey, cfg.ObjectStorageRegion)
	if err != nil {
		t.Fatalf("configure object storage client: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("check configured object storage bucket: %v", err)
	}
}

func TestPutStatAndRemoveWithHostObjectStorage(t *testing.T) {
	if os.Getenv("LV_TEST_OBJECT_STORAGE") != "1" {
		t.Skip("set LV_TEST_OBJECT_STORAGE=1 and LV_ENV_FILE for the host MinIO service")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load object storage configuration: %v", err)
	}
	conn, err := objectstorage.Open(cfg.ObjectStorageEndpoint, cfg.ObjectStorageBucket,
		cfg.ObjectStorageAccessKey, cfg.ObjectStorageSecretKey, cfg.ObjectStorageRegion)
	if err != nil {
		t.Fatalf("configure object storage client: %v", err)
	}
	key := "tests/media-ingest/" + uuid.NewString() + ".png"
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanCancel()
		if err := conn.Remove(cleanCtx, key); err != nil {
			t.Errorf("remove exact test object: %v", err)
		}
	})
	content := "small-media-object"
	digest := sha256.Sum256([]byte(content))
	sha := hex.EncodeToString(digest[:])
	if err := conn.PutIfAbsent(ctx, key, strings.NewReader(content), int64(len(content)), "image/png", sha); err != nil {
		t.Fatalf("put media object: %v", err)
	}
	info, err := conn.Stat(ctx, key)
	if err != nil || info.Size != int64(len(content)) || info.ContentType != "image/png" || info.SHA256 != sha {
		t.Fatalf("stat media object = %+v, %v", info, err)
	}
	if err := conn.PutIfAbsent(ctx, key, strings.NewReader("changed"), 7, "image/png", sha); !errors.Is(err, objectstorage.ErrObjectExists) {
		t.Fatalf("second conditional put error = %v", err)
	}
}
