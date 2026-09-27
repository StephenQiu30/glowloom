package temporalconn_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/otelconn"
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

func traceProbeWorkflow(ctx workflow.Context) (string, error) {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Second,
	})
	var traceID string
	err := workflow.ExecuteActivity(ctx, "trace.probe").Get(ctx, &traceID)
	return traceID, err
}

func traceProbeActivity(ctx context.Context) (string, error) {
	return trace.SpanFromContext(ctx).SpanContext().TraceID().String(), nil
}

func TestWorkflowActivityKeepsParentTrace(t *testing.T) {
	addr := os.Getenv("LV_TEST_TEMPORAL_ADDR")
	namespace := os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	if addr == "" || namespace == "" {
		t.Skip("set LV_TEST_TEMPORAL_ADDR and LV_TEST_TEMPORAL_NAMESPACE for local Temporal")
	}
	provider, shutdown, err := otelconn.Open(t.Context(), "", "lanverse-backend-worker")
	if err != nil {
		t.Fatalf("Open trace provider: %v", err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })
	conn, err := temporalconn.Open(addr, namespace, zap.NewNop(), provider)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(conn.Close)
	queue := "lanverse-test-trace-" + uuid.NewString()
	queueWorker := worker.New(conn.Client, queue, worker.Options{})
	queueWorker.RegisterWorkflow(traceProbeWorkflow)
	queueWorker.RegisterActivityWithOptions(traceProbeActivity, activity.RegisterOptions{Name: "trace.probe"})
	if err := queueWorker.Start(); err != nil {
		t.Fatalf("start trace worker: %v", err)
	}
	t.Cleanup(queueWorker.Stop)

	ctx, parent := provider.Tracer("test").Start(t.Context(), "api request")
	defer parent.End()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	run, err := conn.Client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID: "lanverse-trace-probe-" + uuid.NewString(), TaskQueue: queue,
	}, traceProbeWorkflow)
	if err != nil {
		t.Fatalf("start trace workflow: %v", err)
	}
	var activityTraceID string
	if err := run.Get(ctx, &activityTraceID); err != nil {
		t.Fatalf("run trace workflow: %v", err)
	}
	if activityTraceID != parent.SpanContext().TraceID().String() {
		t.Fatalf("activity trace ID = %s, want parent %s", activityTraceID, parent.SpanContext().TraceID())
	}
}
