package db_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestOpenRejectsMissingDSN(t *testing.T) {
	_, err := db.Open(context.Background(), " ", noop.NewTracerProvider())
	if !errors.Is(err, db.ErrDSNRequired) {
		t.Fatalf("Open() error = %v, want ErrDSNRequired", err)
	}
}

func TestOpenDoesNotExposePassword(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := db.Open(ctx, "postgres://probe:canary-secret@127.0.0.1:1/probe?sslmode=disable", noop.NewTracerProvider())
	if err == nil {
		t.Fatal("Open() succeeded against a closed port")
	}
	if strings.Contains(err.Error(), "canary-secret") {
		t.Fatalf("Open() error exposed a password: %v", err)
	}
}

func TestOpenWithPostgres(t *testing.T) {
	dsn := os.Getenv("LV_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_DB_DSN to a disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	conn, err := db.Open(ctx, dsn, provider)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	var got int
	queryCtx, requestSpan := provider.Tracer("test").Start(ctx, "request")
	if err := conn.DB.WithContext(queryCtx).Raw("SELECT 1").Scan(&got).Error; err != nil {
		t.Fatalf("SELECT 1: %v", err)
	}
	requestSpan.End()
	if got != 1 {
		t.Errorf("SELECT 1 = %d, want 1", got)
	}
	var secret string
	if err := conn.DB.WithContext(queryCtx).Raw("SELECT ?::text", "otel-canary-secret").Scan(&secret).Error; err != nil {
		t.Fatalf("parameterized SELECT: %v", err)
	}
	if secret != "otel-canary-secret" {
		t.Fatalf("parameterized SELECT = %q", secret)
	}
	spans := exporter.GetSpans()
	if len(spans) < 3 {
		t.Fatalf("exported spans = %d, want request and two GORM queries", len(spans))
	}
	var foundQuery bool
	for _, span := range spans {
		if span.Parent.SpanID() != requestSpan.SpanContext().SpanID() {
			continue
		}
		foundQuery = true
		for _, attr := range span.Attributes {
			if strings.Contains(attr.Value.String(), "otel-canary-secret") || strings.Contains(attr.Value.String(), "SELECT 1") {
				t.Fatal("query trace exposed a SQL literal or parameter")
			}
		}
	}
	if !foundQuery {
		t.Fatal("GORM query did not continue the request trace")
	}
}
