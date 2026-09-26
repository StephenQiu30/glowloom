package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
)

func TestHealthz(t *testing.T) {
	router := NewRouter(zap.NewNop(), nil, noop.NewTracerProvider())

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Body.String(), `{"status":"ok"}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestUnknownRouteReturnsNotFound(t *testing.T) {
	router := NewRouter(zap.NewNop(), nil, noop.NewTracerProvider())

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/unknown", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestReadyzReportsDependencyFailureWithoutDetails(t *testing.T) {
	check := func(context.Context) error { return errors.New("secret connection detail") }
	router := NewRouter(zap.NewNop(), check, noop.NewTracerProvider())
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := rec.Body.String(); got != `{"status":"unavailable"}` {
		t.Errorf("body = %s, want generic unavailable status", got)
	}
}

func TestReadyzReportsHealthyDependencies(t *testing.T) {
	check := func(context.Context) error { return nil }
	router := NewRouter(zap.NewNop(), check, noop.NewTracerProvider())
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != `{"status":"ok"}` {
		t.Errorf("status = %d, body = %s, want 200 ok", rec.Code, rec.Body.String())
	}
}

func TestRouterContinuesIncomingTrace(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	router := NewRouter(zap.NewNop(), nil, provider)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", nil)
	req.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}
	if got := spans[0].SpanContext.TraceID().String(); got != "0123456789abcdef0123456789abcdef" {
		t.Errorf("trace ID = %s, want incoming trace ID", got)
	}
	if got := spans[0].Parent.SpanID().String(); got != "0123456789abcdef" {
		t.Errorf("parent span ID = %s, want incoming span ID", got)
	}
}
