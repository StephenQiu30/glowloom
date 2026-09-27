package redisconn_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/redisconn"
)

func TestOpenRejectsMissingURL(t *testing.T) {
	_, err := redisconn.Open(" ")
	if !errors.Is(err, redisconn.ErrURLRequired) {
		t.Fatalf("Open() error = %v, want ErrURLRequired", err)
	}
}

func TestOpenRejectsMalformedURLWithoutExposingPassword(t *testing.T) {
	_, err := redisconn.Open("redis://probe:canary-secret@%invalid/0")
	if !errors.Is(err, redisconn.ErrInvalidURL) {
		t.Fatalf("Open() error = %v, want ErrInvalidURL", err)
	}
	if strings.Contains(err.Error(), "canary-secret") {
		t.Fatalf("Open() error exposed a password: %v", err)
	}
}

func TestPingWithRedis(t *testing.T) {
	url := os.Getenv("LV_TEST_REDIS_URL")
	if url == "" {
		t.Skip("set LV_TEST_REDIS_URL to a disposable Redis database")
	}
	conn, err := redisconn.Open(url)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
}
