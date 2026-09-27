package temporalconn_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	collectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/otelconn"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/temporalconn"
)

func crossLanguageTraceWorkflow(ctx workflow.Context) (map[string]any, error) {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		TaskQueue:           "agent",
		StartToCloseTimeout: 10 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 1},
	})
	payload := map[string]any{
		"operation_id": "trace-probe",
		"output_id":    "trace-probe-output",
		"adapter_key":  "mock",
		"kind":         "text",
		"text":         "trace probe",
		"mock_status":  "passed",
	}
	var result map[string]any
	err := workflow.ExecuteActivity(ctx, "moderation.check", payload).Get(ctx, &result)
	return result, err
}

// TestGoWorkflowToPythonActivityKeepsTraceID runs the real Agent Worker in an
// explicitly isolated Temporal namespace. No Collector is configured for Go;
// the Python Activity span must still inherit the Go request's trace ID.
func TestGoWorkflowToPythonActivityKeepsTraceID(t *testing.T) {
	addr := os.Getenv("LV_TEST_TEMPORAL_ADDR")
	namespace := os.Getenv("LV_TEST_AGENT_TRACE_NAMESPACE")
	redisURL := os.Getenv("LV_TEST_REDIS_URL")
	if addr == "" || namespace == "" || redisURL == "" {
		t.Skip("set LV_TEST_TEMPORAL_ADDR, LV_TEST_AGENT_TRACE_NAMESPACE, and LV_TEST_REDIS_URL for the isolated local trace test")
	}
	if !strings.HasPrefix(namespace, "lanverse-test-") {
		t.Fatal("LV_TEST_AGENT_TRACE_NAMESPACE must name a lanverse-test-* namespace")
	}
	if _, err := exec.LookPath("uv"); err != nil {
		t.Skip("uv is required for the Agent Worker integration test")
	}

	requests := make(chan *collectorpb.ExportTraceServiceRequest, 16)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/traces" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var batch collectorpb.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &batch); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		select {
		case requests <- &batch:
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(receiver.Close)

	provider, shutdown, err := otelconn.Open(t.Context(), "", "lanverse-backend-test")
	if err != nil {
		t.Fatalf("create Go tracer provider: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdown(ctx); err != nil {
			t.Errorf("shutdown Go tracer provider: %v", err)
		}
	})
	conn, err := temporalconn.Open(addr, namespace, zap.NewNop(), provider)
	if err != nil {
		t.Fatalf("open Temporal client: %v", err)
	}
	t.Cleanup(conn.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("ping isolated Temporal namespace: %v", err)
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	processCtx, cancelProcess := context.WithCancel(context.Background())
	t.Cleanup(cancelProcess)
	cmd := exec.CommandContext(processCtx, "uv", "run", "--frozen", "python", "-m", "app.main_worker")
	cmd.Dir = filepath.Join(repoRoot, "agent")
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"TMPDIR=" + os.Getenv("TMPDIR"),
		"PYTHONUNBUFFERED=1",
		"LV_TEMPORAL_ADDR=" + addr,
		"LV_TEMPORAL_NAMESPACE=" + namespace,
		"LV_REDIS_URL=" + redisURL,
		"LV_OTEL_ENDPOINT=" + receiver.URL,
	}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("start Agent Worker: %v", err)
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	stopped := false
	stopAgent := func() error {
		if stopped {
			return nil
		}
		if err := cmd.Process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
			_ = cmd.Process.Kill()
			<-wait
			stopped = true
			return err
		}
		stopped = true
		select {
		case err := <-wait:
			return err
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			<-wait
			return errors.New("Agent Worker did not stop after interrupt")
		}
	}
	t.Cleanup(func() { _ = stopAgent() })

	readyCtx, readyCancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer readyCancel()
	for {
		select {
		case err := <-wait:
			stopped = true
			t.Fatalf("Agent Worker exited before polling: %v", err)
		default:
		}
		probeCtx, probeCancel := context.WithTimeout(readyCtx, 2*time.Second)
		queue, err := conn.Client.DescribeTaskQueue(probeCtx, "agent", enums.TASK_QUEUE_TYPE_ACTIVITY)
		probeCancel()
		if err == nil && len(queue.Pollers) > 0 {
			break
		}
		select {
		case <-readyCtx.Done():
			t.Fatalf("Agent Worker did not poll the isolated activity queue: %v", readyCtx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}

	queue := "lanverse-test-go-python-trace-" + uuid.NewString()
	flowWorker := worker.New(conn.Client, queue, worker.Options{})
	flowWorker.RegisterWorkflow(crossLanguageTraceWorkflow)
	if err := flowWorker.Start(); err != nil {
		t.Fatalf("start Go workflow worker: %v", err)
	}
	t.Cleanup(flowWorker.Stop)

	ctx, parent := provider.Tracer("test").Start(t.Context(), "api request")
	defer parent.End()
	if !parent.SpanContext().IsValid() {
		t.Fatal("Go request did not create a valid trace without a Collector")
	}
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	run, err := conn.Client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID: "lanverse-test-go-python-trace-" + uuid.NewString(), TaskQueue: queue,
	}, crossLanguageTraceWorkflow)
	if err != nil {
		t.Fatalf("start Go trace workflow: %v", err)
	}
	var result map[string]any
	if err := run.Get(ctx, &result); err != nil {
		t.Fatalf("run Go trace workflow: %v", err)
	}
	if result["status"] != "passed" {
		t.Fatalf("Agent moderation status = %v, want passed", result["status"])
	}
	if err := stopAgent(); err != nil {
		t.Fatalf("stop Agent Worker and flush trace export: %v", err)
	}
	if !hasPythonActivityTrace(requests, parent.SpanContext()) {
		t.Fatalf("Agent RunActivity span did not inherit Go request trace ID %s", parent.SpanContext().TraceID())
	}
}

func hasPythonActivityTrace(requests <-chan *collectorpb.ExportTraceServiceRequest, parent trace.SpanContext) bool {
	wantTraceID := parent.TraceID()
	for {
		select {
		case batch := <-requests:
			for _, resource := range batch.ResourceSpans {
				for _, scope := range resource.ScopeSpans {
					for _, span := range scope.Spans {
						if span.Name == "RunActivity:moderation.check" && bytes.Equal(span.TraceId, wantTraceID[:]) {
							return true
						}
					}
				}
			}
		default:
			return false
		}
	}
}
