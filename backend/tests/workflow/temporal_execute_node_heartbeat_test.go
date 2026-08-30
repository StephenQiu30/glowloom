package workflow_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
	temporalworkflow "go.temporal.io/sdk/workflow"

	temporaladapter "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowdomain "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const slowSubmitExecuteNodeWorkflowName = "lanverse.test.workflow.slow-submit-execute-node"

func TestExecuteNodeActivityHeartbeatsDuringSlowSynchronousSubmit(t *testing.T) {
	activities := &slowSubmitRuntimeActivities{delay: 350 * time.Millisecond}
	var suite testsuite.WorkflowTestSuite
	environment := suite.NewTestWorkflowEnvironment()
	environment.RegisterWorkflowWithOptions(
		slowSubmitExecuteNodeWorkflow,
		temporalworkflow.RegisterOptions{Name: slowSubmitExecuteNodeWorkflowName},
	)
	environment.RegisterActivityWithOptions(
		temporaladapter.NewExecuteNodeActivityHandler(activities.ExecuteNode),
		activity.RegisterOptions{Name: temporaladapter.ExecuteNodeActivityName},
	)

	environment.ExecuteWorkflow(slowSubmitExecuteNodeWorkflowName)
	if err := environment.GetWorkflowError(); err != nil {
		t.Fatalf("slow synchronous Submit should survive the heartbeat timeout: %v", err)
	}
	if calls := activities.executeCalls.Load(); calls != 1 {
		t.Fatalf("slow synchronous Submit calls = %d, want exactly 1", calls)
	}
}

func slowSubmitExecuteNodeWorkflow(ctx temporalworkflow.Context) error {
	activityContext := temporalworkflow.WithActivityOptions(ctx, temporalworkflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Second,
		HeartbeatTimeout:    100 * time.Millisecond,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Millisecond,
			BackoffCoefficient: 1,
			MaximumInterval:    time.Millisecond,
			MaximumAttempts:    2,
		},
	})
	var result workflowdomain.NodeActivityResult
	return temporalworkflow.ExecuteActivity(
		activityContext,
		temporaladapter.ExecuteNodeActivityName,
		workflowdomain.NodeActivityCommand{},
	).Get(ctx, &result)
}

type slowSubmitRuntimeActivities struct {
	delay        time.Duration
	executeCalls atomic.Int32
}

func (activities *slowSubmitRuntimeActivities) ExecuteNode(
	ctx context.Context,
	_ workflowdomain.NodeActivityCommand,
) (workflowdomain.NodeActivityResult, error) {
	activities.executeCalls.Add(1)
	select {
	case <-time.After(activities.delay):
		return workflowdomain.NodeActivityResult{Status: "SUCCEEDED"}, nil
	case <-ctx.Done():
		return workflowdomain.NodeActivityResult{}, ctx.Err()
	}
}
