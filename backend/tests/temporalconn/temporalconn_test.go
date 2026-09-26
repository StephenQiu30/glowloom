package temporalconn_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/temporalconn"
)

func TestOpenRejectsMissingConfig(t *testing.T) {
	_, err := temporalconn.Open("", "lanverse-local", zap.NewNop(), noop.NewTracerProvider())
	if !errors.Is(err, temporalconn.ErrAddrRequired) {
		t.Fatalf("Open() error = %v, want ErrAddrRequired", err)
	}
	_, err = temporalconn.Open("127.0.0.1:7233", "", zap.NewNop(), noop.NewTracerProvider())
	if !errors.Is(err, temporalconn.ErrNamespaceRequired) {
		t.Fatalf("Open() error = %v, want ErrNamespaceRequired", err)
	}
	_, err = temporalconn.Open("127.0.0.1:7233", "lanverse-local", zap.NewNop(), nil)
	if !errors.Is(err, temporalconn.ErrTracerProviderRequired) {
		t.Fatalf("Open() error = %v, want ErrTracerProviderRequired", err)
	}
}

func TestOpenSurvivesUnavailableServer(t *testing.T) {
	conn, err := temporalconn.Open("127.0.0.1:1", "lanverse-local", zap.NewNop(), noop.NewTracerProvider())
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
	conn, err := temporalconn.Open(addr, namespace, zap.NewNop(), noop.NewTracerProvider())
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
