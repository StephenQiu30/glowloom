package temporalconn

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestOpenRejectsMissingConfig(t *testing.T) {
	_, err := Open("", "lanverse-local", zap.NewNop())
	if !errors.Is(err, ErrAddrRequired) {
		t.Fatalf("Open() error = %v, want ErrAddrRequired", err)
	}
	_, err = Open("127.0.0.1:7233", "", zap.NewNop())
	if !errors.Is(err, ErrNamespaceRequired) {
		t.Fatalf("Open() error = %v, want ErrNamespaceRequired", err)
	}
}

func TestOpenSurvivesUnavailableServer(t *testing.T) {
	conn, err := Open("127.0.0.1:1", "lanverse-local", zap.NewNop())
	if err != nil {
		t.Fatalf("Open() error = %v, want a lazy client", err)
	}
	t.Cleanup(conn.Close)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err == nil {
		t.Fatal("Ping() succeeded against a closed port")
	}
}

func TestPingWithTemporal(t *testing.T) {
	addr := os.Getenv("LV_TEST_TEMPORAL_ADDR")
	namespace := os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	if addr == "" || namespace == "" {
		t.Skip("set LV_TEST_TEMPORAL_ADDR and LV_TEST_TEMPORAL_NAMESPACE for local Temporal")
	}
	conn, err := Open(addr, namespace, zap.NewNop())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(conn.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
}
