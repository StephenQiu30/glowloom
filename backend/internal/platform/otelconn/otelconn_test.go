package otelconn

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	collectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestOpenRejectsInvalidEndpoint(t *testing.T) {
	for _, endpoint := range []string{"ftp://127.0.0.1:4318", "http://user:secret@127.0.0.1:4318", "http://127.0.0.1:4318/custom", "http://127.0.0.1:4318?token=secret"} {
		_, _, err := Open(t.Context(), endpoint, "lanverse-backend-api")
		if !errors.Is(err, ErrInvalidEndpoint) {
			t.Errorf("Open(%q) error = %v, want ErrInvalidEndpoint", endpoint, err)
		}
	}
}

func TestOpenWithoutEndpointDisablesExport(t *testing.T) {
	provider, shutdown, err := Open(t.Context(), "", "lanverse-backend-api")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	_, span := provider.Tracer("test").Start(t.Context(), "probe")
	if span.IsRecording() {
		t.Fatal("disabled provider recorded a span")
	}
	span.End()
	if err := shutdown(t.Context()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestOpenExportsTraceOverHTTP(t *testing.T) {
	requests := make(chan *collectorpb.ExportTraceServiceRequest, 1)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/traces" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var batch collectorpb.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &batch); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- &batch
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(receiver.Close)

	provider, shutdown, err := Open(t.Context(), receiver.URL, "lanverse-backend-api")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	_, span := provider.Tracer("test").Start(t.Context(), "probe")
	span.End()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	select {
	case batch := <-requests:
		if len(batch.ResourceSpans) != 1 || len(batch.ResourceSpans[0].ScopeSpans) != 1 || len(batch.ResourceSpans[0].ScopeSpans[0].Spans) != 1 {
			t.Fatalf("unexpected exported span count: %v", batch)
		}
		if got := batch.ResourceSpans[0].ScopeSpans[0].Spans[0].Name; got != "probe" {
			t.Errorf("span name = %q, want probe", got)
		}
		foundService := false
		for _, attr := range batch.ResourceSpans[0].Resource.Attributes {
			if attr.Key == "service.name" && attr.Value.GetStringValue() == "lanverse-backend-api" {
				foundService = true
			}
		}
		if !foundService {
			t.Error("exported span has no lanverse service.name")
		}
	default:
		t.Fatal("OTLP receiver got no trace request")
	}
}
