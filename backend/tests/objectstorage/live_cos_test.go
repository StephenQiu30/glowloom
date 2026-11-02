package objectstorage_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

func TestLiveCOSConnectivity(t *testing.T) {
	// Look for .env in potential relative paths
	envPath := "../../../.env"
	if _, err := os.Stat(envPath); err != nil {
		envPath = "../../.env"
	}
	if _, err := os.Stat(envPath); err != nil {
		t.Skip("skipping live COS test: .env not found")
	}

	os.Setenv("GLOWLOOM_ENV_FILE", envPath)
	os.Setenv("GLOWLOOM_CANVAS_RUNTIME_URL", "http://127.0.0.1:4400")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	if cfg.ObjectStorageProvider != "cos" || cfg.ObjectStorageAccessKey == "" {
		t.Skip("skipping live COS test: COS provider or credentials not configured")
	}

	storage, err := objectstorage.OpenFromConfig(cfg)
	if err != nil {
		t.Fatalf("OpenFromConfig failed: %v", err)
	}
	defer storage.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 1. Ping test
	t.Log("Testing storage.Ping()...")
	if err := storage.Ping(ctx); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
	t.Log("✓ Ping succeeded!")

	// 2. Put / PutIfAbsent test
	probeKey := "probe/connectivity-test.txt"
	probeData := "glowloom-cos-live-verification-" + time.Now().UTC().Format(time.RFC3339Nano)
	sum := sha256.Sum256([]byte(probeData))
	probeSHA := hex.EncodeToString(sum[:])

	t.Logf("Testing storage.PutIfAbsent(%s)...", probeKey)
	if err := storage.PutIfAbsent(ctx, probeKey, strings.NewReader(probeData), int64(len(probeData)), "text/plain", probeSHA); err != nil {
		t.Fatalf("PutIfAbsent failed: %v", err)
	}
	t.Log("✓ PutIfAbsent succeeded!")

	// Ensure cleanup
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = storage.Remove(cleanupCtx, probeKey)
		t.Log("✓ Cleanup Remove completed")
	}()

	// 3. Stat test
	t.Logf("Testing storage.Stat(%s)...", probeKey)
	info, err := storage.Stat(ctx, probeKey)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Size != int64(len(probeData)) {
		t.Fatalf("Stat size mismatch: got %d, want %d", info.Size, len(probeData))
	}
	t.Logf("✓ Stat succeeded: size=%d, contentType=%s, sha256=%s", info.Size, info.ContentType, info.SHA256)

	// 4. Get test
	t.Logf("Testing storage.Get(%s)...", probeKey)
	rc, err := storage.Get(ctx, probeKey)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	body, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(body) != probeData {
		t.Fatalf("Body mismatch: got %q, want %q", string(body), probeData)
	}
	t.Logf("✓ Get succeeded, read %d bytes exactly matching probe data", len(body))

	// 5. PresignGet test
	t.Logf("Testing storage.PresignGet(%s)...", probeKey)
	signedURL, err := storage.PresignGet(ctx, probeKey, 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignGet failed: %v", err)
	}
	if signedURL == "" || !strings.Contains(signedURL, cfg.ObjectStorageBucket) {
		t.Fatalf("Unexpected signedURL: %s", signedURL)
	}
	t.Logf("✓ PresignGet succeeded: %s", signedURL)
}
