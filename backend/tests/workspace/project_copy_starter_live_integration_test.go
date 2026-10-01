package workspace_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"

	copyflow "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/workflow"
	copyapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

// isolatedCopyClient uses the actual server and SDK on a queue without business workers.
type isolatedCopyClient struct {
	client.Client
	queue string
}

func (c isolatedCopyClient) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow any, args ...any) (client.WorkflowRun, error) {
	options.TaskQueue = c.queue
	return c.Client.ExecuteWorkflow(ctx, options, workflow, args...)
}

func TestProjectCopyRealTemporalRetrySignalsExistingWorkflow(t *testing.T) {
	address, namespace := os.Getenv("LV_TEST_TEMPORAL_ADDR"), os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	if address == "" || namespace == "" {
		t.Skip("set LV_TEST_TEMPORAL_ADDR and LV_TEST_TEMPORAL_NAMESPACE")
	}
	c, err := client.Dial(client.Options{HostPort: address, Namespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	job, org := uuid.New(), uuid.New()
	id := "project-copy/" + job.String()
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if err := c.TerminateWorkflow(cleanup, id, "", "isolated copy starter acceptance finished"); err != nil {
			t.Error(err)
		}
	})
	starter := copyflow.NewProjectCopyStarter(isolatedCopyClient{Client: c, queue: "copy-starter-test-" + job.String()})
	delivery := copyapp.ProjectCopyDelivery{OrgID: org, JobID: job, Action: "requested"}
	if err := starter.Deliver(ctx, delivery); err != nil {
		t.Fatal(err)
	}
	first, err := c.DescribeWorkflowExecution(ctx, id, "")
	if err != nil {
		t.Fatal(err)
	}
	delivery.Action = "retry"
	if err := starter.Deliver(ctx, delivery); err != nil {
		t.Fatal(err)
	}
	latest, err := c.DescribeWorkflowExecution(ctx, id, "")
	if err != nil || latest.WorkflowExecutionInfo.Execution.RunId != first.WorkflowExecutionInfo.Execution.RunId {
		t.Fatal("retry replaced the existing workflow", err)
	}
	history := c.GetWorkflowHistory(ctx, id, first.WorkflowExecutionInfo.Execution.RunId, false, enums.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	starts, signals := 0, 0
	for history.HasNext() {
		event, err := history.Next()
		if err != nil {
			t.Fatal(err)
		}
		if event.EventType == enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED {
			starts++
		}
		if event.EventType == enums.EVENT_TYPE_WORKFLOW_EXECUTION_SIGNALED {
			if event.GetWorkflowExecutionSignaledEventAttributes().SignalName != copyflow.CopySignal {
				t.Fatal("wrong copy command signal")
			}
			signals++
		}
	}
	if starts != 1 || signals != 1 {
		t.Fatalf("real SDK retry must signal the existing run: starts=%d signals=%d", starts, signals)
	}
}
