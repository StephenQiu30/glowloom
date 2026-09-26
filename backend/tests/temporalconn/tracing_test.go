package temporalconn_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/temporalconn"
)

func TestQueryWorkflowKeepsParentTrace(t *testing.T) {
	addr := os.Getenv("LV_TEST_TEMPORAL_ADDR")
	namespace := os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	if addr == "" || namespace == "" {
		t.Skip("set LV_TEST_TEMPORAL_ADDR and LV_TEST_TEMPORAL_NAMESPACE for local Temporal")
	}
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	conn, err := temporalconn.Open(addr, namespace, zap.NewNop(), provider)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(conn.Close)
	pingCtx, pingCancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer pingCancel()
	if err := conn.Ping(pingCtx); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}

	ctx, parent := provider.Tracer("test").Start(t.Context(), "api request")
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err = conn.Client.QueryWorkflow(ctx, "lanverse-trace-probe-nonexistent", "", "trace-probe")
	parent.End()
	if err == nil {
		t.Fatal("QueryWorkflow() found a nonexistent workflow")
	}

	for _, span := range recorder.Ended() {
		if strings.HasPrefix(span.Name(), "QueryWorkflow:") {
			if span.SpanContext().TraceID() != parent.SpanContext().TraceID() || span.Parent().SpanID() != parent.SpanContext().SpanID() {
				t.Fatalf("workflow span is not a child of the request: trace=%s parent=%s", span.SpanContext().TraceID(), span.Parent().SpanID())
			}
			return
		}
	}
	t.Fatal("missing Temporal QueryWorkflow span")
}
