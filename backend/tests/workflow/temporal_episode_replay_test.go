package workflow_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"

	temporaladapter "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowdomain "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestEpisodeWorkflowCompletesOnRealTemporalAndReplaysHistory(t *testing.T) {
	address := os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS")
	if address == "" {
		t.Skip("set LANVERSE_TEST_TEMPORAL_ADDRESS to run the real Temporal Episode Workflow journey")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	temporalClient, err := client.Dial(client.Options{HostPort: address, Namespace: "default"})
	if err != nil {
		t.Fatalf("connect Temporal: %v", err)
	}
	t.Cleanup(temporalClient.Close)

	request := episodeWorkflowStartRequest()
	request.WorkflowID = "lanverse:replay:" + uuid.NewString()
	request.WorkflowRunID = uuid.NewString()
	taskQueue := "lanverse-episode-replay-" + uuid.NewString()
	plan := temporaladapter.ExecutionPlan{
		WorkflowRunID: request.WorkflowRunID, DefinitionVersionID: request.DefinitionVersionID,
		RunInputSnapshotID: request.RunInputSnapshotID, DefinitionContentHash: request.DefinitionContentHash,
		InputSnapshotHash: request.InputSnapshotHash,
		Nodes: []temporaladapter.ExecutionNode{
			{NodeRunID: uuid.NewString(), NodeID: "script", Executor: "workflow.input.script_revision", RiskLevel: "low"},
			{NodeRunID: uuid.NewString(), NodeID: "review", Executor: "gate.production_bible_review", RiskLevel: "human_gate"},
			{NodeRunID: uuid.NewString(), NodeID: "export", Executor: "activity.storyboard_export", RiskLevel: "low"},
		},
	}

	runtimeWorker := worker.New(temporalClient, taskQueue, worker.Options{})
	runtimeWorker.RegisterWorkflowWithOptions(
		temporaladapter.EpisodeProductionWorkflow,
		temporalworkflow.RegisterOptions{Name: temporaladapter.EpisodeProductionWorkflowName},
	)
	runtimeWorker.RegisterActivityWithOptions(
		func(context.Context, workflowdomain.StartRequest) (temporaladapter.ExecutionPlan, error) {
			return plan, nil
		},
		activity.RegisterOptions{Name: temporaladapter.LoadExecutionPlanActivityName},
	)
	runtimeWorker.RegisterActivityWithOptions(
		func(context.Context, temporaladapter.NodeActivityCommand) (temporaladapter.NodeActivityResult, error) {
			return temporaladapter.NodeActivityResult{Status: "SUCCEEDED"}, nil
		},
		activity.RegisterOptions{Name: temporaladapter.ExecuteNodeActivityName},
	)
	runtimeWorker.RegisterActivityWithOptions(
		func(context.Context, temporaladapter.NodeActivityCommand) error { return nil },
		activity.RegisterOptions{Name: temporaladapter.OpenHumanGateActivityName},
	)
	runtimeWorker.RegisterActivityWithOptions(
		func(context.Context, temporaladapter.ApplyHumanGateCommand) error { return nil },
		activity.RegisterOptions{Name: temporaladapter.ApplyHumanGateActivityName},
	)
	runtimeWorker.RegisterActivityWithOptions(
		func(context.Context, temporaladapter.CompleteRunCommand) error { return nil },
		activity.RegisterOptions{Name: temporaladapter.CompleteRunActivityName},
	)
	if err = runtimeWorker.Start(); err != nil {
		t.Fatalf("start Temporal worker: %v", err)
	}
	t.Cleanup(runtimeWorker.Stop)

	run, err := temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID: request.WorkflowID, TaskQueue: taskQueue,
	}, temporaladapter.EpisodeProductionWorkflowName, request)
	if err != nil {
		t.Fatalf("start Episode Workflow: %v", err)
	}
	if err = temporalClient.SignalWorkflow(ctx, run.GetID(), run.GetRunID(), temporaladapter.HumanGateSignalName, temporaladapter.HumanGateSignal{
		WorkflowRunID: request.WorkflowRunID, NodeRunID: plan.Nodes[1].NodeRunID,
		SignalReceiptID: uuid.NewString(), Decision: "APPROVED",
	}); err != nil {
		t.Fatalf("signal human gate: %v", err)
	}
	var result temporaladapter.RunResult
	if err = run.Get(ctx, &result); err != nil {
		t.Fatalf("wait Episode Workflow: %v", err)
	}
	if result.WorkflowRunID != request.WorkflowRunID || result.Status != "SUCCEEDED" {
		t.Fatalf("workflow result = %#v", result)
	}

	iterator := temporalClient.GetWorkflowHistory(ctx, run.GetID(), run.GetRunID(), false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	history := &historypb.History{}
	for iterator.HasNext() {
		event, nextErr := iterator.Next()
		if nextErr != nil {
			t.Fatalf("read workflow history: %v", nextErr)
		}
		history.Events = append(history.Events, event)
	}
	if len(history.Events) == 0 {
		t.Fatal("Temporal returned an empty workflow history")
	}
	replayer := worker.NewWorkflowReplayer()
	replayer.RegisterWorkflowWithOptions(
		temporaladapter.EpisodeProductionWorkflow,
		temporalworkflow.RegisterOptions{Name: temporaladapter.EpisodeProductionWorkflowName},
	)
	if err = replayer.ReplayWorkflowHistory(nil, history); err != nil {
		t.Fatalf("replay Episode Workflow history: %v", err)
	}
}
