package objectstorage_test

import (
	"context"
	"os"
	"testing"
	"time"

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
