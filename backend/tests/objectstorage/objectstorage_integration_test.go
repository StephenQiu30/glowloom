package objectstorage_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

func TestPingWithHostObjectStorage(t *testing.T) {
	if os.Getenv("GLOWLOOM_TEST_OBJECT_STORAGE") != "1" {
		t.Skip("set GLOWLOOM_TEST_OBJECT_STORAGE=1 and GLOWLOOM_ENV_FILE to verify the host object storage service")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load object storage configuration: %v", err)
	}
	conn, err := objectstorage.OpenProfiles(*cfg.ObjectStorage)
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
	if os.Getenv("GLOWLOOM_TEST_OBJECT_STORAGE") != "1" {
		t.Skip("set GLOWLOOM_TEST_OBJECT_STORAGE=1 and GLOWLOOM_ENV_FILE for the dedicated COS fixture")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load object storage configuration: %v", err)
	}
	conn, err := objectstorage.OpenProfiles(*cfg.ObjectStorage)
	if err != nil {
		t.Fatalf("configure object storage client: %v", err)
	}
	key := objectstorage.Ref{ProfileID: cfg.ObjectStorage.DefaultWriteProfile, Bucket: cfg.ObjectStorage.Profiles[cfg.ObjectStorage.DefaultWriteProfile].Bucket, Key: "tests/media-ingest/" + uuid.NewString() + ".png"}
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
	grant, err := conn.PresignGet(ctx, key, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, grant.URL, nil)
	if err != nil {
		t.Fatal("build signed request")
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("read signed object")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK || string(data) != content {
		t.Fatalf("signed read did not return the original object: status %d, error %v", response.StatusCode, err)
	}
}
