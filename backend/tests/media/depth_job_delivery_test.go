package media_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"

	inboxpg "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/postgres"
	inboxapp "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	toolevent "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/event"
	toolflow "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/workflow"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

type depthDeliveryRecorder struct {
	calls int
	fail  bool
}

func (s *depthDeliveryRecorder) Deliver(context.Context, toolapp.DepthDelivery) error {
	s.calls++
	if s.fail {
		return errors.New("controlled scheduling failure")
	}
	return nil
}

func TestDepthJobPGExactOutboxAndInboxDelivery(t *testing.T) {
	f := depthHTTP(t)
	j := f.create(t)
	var row struct {
		ID           uuid.UUID
		PartitionKey string
		Payload      []byte
	}
	if err := f.db.Raw(`SELECT id,partition_key,payload FROM infra.outbox WHERE topic=? AND partition_key=?`, toolevent.DepthTopic, j.ID.String()).Scan(&row).Error; err != nil || row.ID == uuid.Nil {
		t.Fatal("missing exact depth outbox", err)
	}
	starter := &depthDeliveryRecorder{fail: true}
	h := toolevent.NewDepthHandler(inboxpg.NewStore(f.db), f.store, starter)
	record := inboxapp.Record{Topic: toolevent.DepthTopic, Key: []byte(row.PartitionKey), Value: row.Payload}
	var body map[string]any
	if err := json.Unmarshal(row.Payload, &body); err != nil {
		t.Fatal(err)
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatal("outbox data missing")
	}
	data["project_id"] = uuid.NewString()
	injected, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(t.Context(), inboxapp.Record{Topic: record.Topic, Key: record.Key, Value: injected}); !errors.Is(err, toolapp.ErrInvalidDepthInput) {
		t.Fatal("forged outbox command acknowledged", err)
	}
	if starter.calls != 0 {
		t.Fatal("forged payload reached physical scheduler")
	}
	if err := h.Handle(t.Context(), record); err == nil || starter.calls != 1 {
		t.Fatal("scheduling failure acknowledged", err)
	}
	starter.fail = false
	if err := h.Handle(t.Context(), record); err != nil || starter.calls != 2 {
		t.Fatal("failed delivery could not be recovered", err)
	}
	if err := h.Handle(t.Context(), record); err != nil || starter.calls != 2 {
		t.Fatal("duplicate permanent inbox scheduling", err)
	}
}

type isolatedDepthClient struct {
	client.Client
	queue string
}

func (c isolatedDepthClient) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, w any, args ...any) (client.WorkflowRun, error) {
	options.TaskQueue = c.queue
	return c.Client.ExecuteWorkflow(ctx, options, w, args...)
}

func TestDepthJobRealTemporalDuplicateAndCancellationCommands(t *testing.T) {
	address, namespace := os.Getenv("LV_TEST_TEMPORAL_ADDR"), os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	if address == "" || namespace == "" {
		t.Skip("configure explicitly authorized isolated workflow queue on actual local Temporal")
	}
	c, err := client.Dial(client.Options{HostPort: address, Namespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	job, key := uuid.New(), uuid.New()
	d := toolapp.DepthDelivery{DepthWorkID: toolapp.DepthWorkID{JobID: job, Attempt: 1}, RequestID: key, Action: "start"}
	base := "media-depth/" + job.String() + "/" + strconv.Itoa(d.Attempt)
	control := base + "/cancel/" + key.String()
	t.Cleanup(func() {
		end, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		for _, id := range []string{base, control} {
			if err := c.TerminateWorkflow(end, id, "", "task-owned depth starter verification finished"); err != nil {
				t.Error(err)
			}
		}
	})
	start := toolflow.NewDepthStarter(isolatedDepthClient{Client: c, queue: "nativeDepth-test-" + job.String()})
	if err := start.Deliver(ctx, d); err != nil {
		t.Fatal(err)
	}
	first, err := c.DescribeWorkflowExecution(ctx, base, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := start.Deliver(ctx, d); err != nil {
		t.Fatal(err)
	}
	last, err := c.DescribeWorkflowExecution(ctx, base, "")
	if err != nil || first.WorkflowExecutionInfo.Execution.RunId != last.WorkflowExecutionInfo.Execution.RunId {
		t.Fatal("duplicate native start created another run", err)
	}
	d.Action = "cancel"
	if err := start.Deliver(ctx, d); err != nil {
		t.Fatal(err)
	}
	cleanup, err := c.DescribeWorkflowExecution(ctx, control, "")
	if err != nil {
		t.Fatal("post-review cancellation has no cleanup workflow", err)
	}
	if err := start.Deliver(ctx, d); err != nil {
		t.Fatal(err)
	}
	same, err := c.DescribeWorkflowExecution(ctx, control, "")
	if err != nil || cleanup.WorkflowExecutionInfo.Execution.RunId != same.WorkflowExecutionInfo.Execution.RunId {
		t.Fatal("duplicate cancellation created another cleanup", err)
	}
	history := c.GetWorkflowHistory(ctx, base, first.WorkflowExecutionInfo.Execution.RunId, false, enums.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	starts, signals := 0, 0
	for history.HasNext() {
		e, err := history.Next()
		if err != nil {
			t.Fatal(err)
		}
		switch e.EventType {
		case enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED:
			starts++
		case enums.EVENT_TYPE_WORKFLOW_EXECUTION_SIGNALED:
			if e.GetWorkflowExecutionSignaledEventAttributes().SignalName != "cancel" {
				t.Fatal("wrong native stop signal")
			}
			signals++
		}
	}
	if starts != 1 || signals != 2 {
		t.Fatalf("actual SDK signal/duplicate semantics starts=%d signals=%d", starts, signals)
	}
}
