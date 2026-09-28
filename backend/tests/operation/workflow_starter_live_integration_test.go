package operation_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"

	operationflow "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/workflow"
)

func TestOperationStarterKeepsOneWorkflowOnLocalTemporal(t *testing.T) {
	addr, namespace := os.Getenv("LV_TEST_TEMPORAL_ADDR"), os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	if addr == "" || namespace == "" {
		t.Skip("set LV_TEST_TEMPORAL_ADDR and LV_TEST_TEMPORAL_NAMESPACE for local Temporal")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	workflowClient, err := client.Dial(client.Options{HostPort: addr, Namespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(workflowClient.Close)
	id := uuid.New()
	workflowID := "operation/" + id.String()
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = workflowClient.TerminateWorkflow(cleanupCtx, workflowID, "", "test cleanup")
	})
	starter := operationflow.NewStarter(workflowClient)
	if err := starter.StartOperation(ctx, id); err != nil {
		t.Fatalf("start workflow: %v", err)
	}
	first, err := workflowClient.DescribeWorkflowExecution(ctx, workflowID, "")
	if err != nil {
		t.Fatalf("describe first run: %v", err)
	}
	if err := starter.StartOperation(ctx, id); err != nil {
		t.Fatalf("repeat running workflow: %v", err)
	}
	if err := workflowClient.TerminateWorkflow(ctx, workflowID, "", "test closed-ID reuse"); err != nil {
		t.Fatalf("terminate first run: %v", err)
	}
	for {
		closed, describeErr := workflowClient.DescribeWorkflowExecution(ctx, workflowID, "")
		if describeErr != nil {
			t.Fatalf("describe terminated run: %v", describeErr)
		}
		if closed.WorkflowExecutionInfo.Status == enums.WORKFLOW_EXECUTION_STATUS_TERMINATED {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("workflow did not terminate: %v", ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
	if err := starter.StartOperation(ctx, id); err != nil {
		t.Fatalf("repeat closed workflow: %v", err)
	}
	last, err := workflowClient.DescribeWorkflowExecution(ctx, workflowID, "")
	if err != nil {
		t.Fatalf("describe last run: %v", err)
	}
	if first.WorkflowExecutionInfo.Execution.RunId != last.WorkflowExecutionInfo.Execution.RunId {
		t.Fatalf("workflow ID reused after close: first=%v last=%v", first.WorkflowExecutionInfo.Execution.RunId, last.WorkflowExecutionInfo.Execution.RunId)
	}
}
