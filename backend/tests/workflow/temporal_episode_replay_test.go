package workflow_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"

	temporaladapter "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
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

	runtime, err := temporaladapter.New(temporaladapter.Config{
		Address: address, Namespace: "default", TaskQueue: taskQueue,
	})
	if err != nil {
		t.Fatalf("connect Temporal runtime: %v", err)
	}
	t.Cleanup(runtime.Close)
	runtimeWorker, err := runtime.NewWorker(&replayRuntimeActivities{plan: plan})
	if err != nil {
		t.Fatalf("register Temporal worker: %v", err)
	}
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
	signalIntent := workflowdomain.SignalIntent{
		ID: uuid.NewString(), TemporalWorkflowID: run.GetID(), SignalID: uuid.NewString(),
		WorkflowRunID: request.WorkflowRunID, NodeRunID: plan.Nodes[1].NodeRunID, Decision: "approved",
	}
	signalRequest, err := workflowapp.NewSignalRequest(signalIntent)
	if err != nil {
		t.Fatalf("build human gate signal: %v", err)
	}
	signaled, err := runtime.Signal(ctx, signalRequest)
	if err != nil || signaled.Outcome != workflowdomain.SignalOutcomeSignaled || signaled.ObservedInputHash != signalRequest.InputHash {
		t.Fatalf("signal human gate: observation=%#v err=%v", signaled, err)
	}
	alreadyApplied, err := runtime.Signal(ctx, signalRequest)
	if err != nil || alreadyApplied.Outcome != workflowdomain.SignalOutcomeAlreadyApplied ||
		alreadyApplied.ObservedInputHash != signalRequest.InputHash {
		t.Fatalf("reconcile repeated human gate signal: observation=%#v err=%v", alreadyApplied, err)
	}
	conflictingIntent := signalIntent
	conflictingIntent.Decision = "rejected"
	conflictingRequest, err := workflowapp.NewSignalRequest(conflictingIntent)
	if err != nil {
		t.Fatalf("build conflicting human gate signal: %v", err)
	}
	conflict, err := runtime.Signal(ctx, conflictingRequest)
	if err != nil || conflict.Outcome != workflowdomain.SignalOutcomeConflict ||
		conflict.ObservedInputHash != signalRequest.InputHash {
		t.Fatalf("reject drifted repeated human gate signal: observation=%#v err=%v", conflict, err)
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
	humanGateSignalEvents := 0
	for iterator.HasNext() {
		event, nextErr := iterator.Next()
		if nextErr != nil {
			t.Fatalf("read workflow history: %v", nextErr)
		}
		history.Events = append(history.Events, event)
		if attributes := event.GetWorkflowExecutionSignaledEventAttributes(); attributes != nil &&
			attributes.GetSignalName() == temporaladapter.HumanGateSignalName {
			humanGateSignalEvents++
		}
	}
	if len(history.Events) == 0 {
		t.Fatal("Temporal returned an empty workflow history")
	}
	if humanGateSignalEvents != 1 {
		t.Fatalf("Temporal human gate signal events = %d, want 1", humanGateSignalEvents)
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

type replayRuntimeActivities struct {
	plan temporaladapter.ExecutionPlan
}

func (activities *replayRuntimeActivities) LoadExecutionPlan(
	context.Context,
	workflowdomain.StartRequest,
) (workflowdomain.ExecutionPlan, error) {
	return activities.plan, nil
}

func (*replayRuntimeActivities) ExecuteNode(
	context.Context,
	workflowdomain.NodeActivityCommand,
) (workflowdomain.NodeActivityResult, error) {
	return successfulNodeActivityResult(), nil
}

func (*replayRuntimeActivities) OpenHumanGate(context.Context, workflowdomain.NodeActivityCommand) error {
	return nil
}

func (*replayRuntimeActivities) ApplyHumanGate(context.Context, workflowdomain.ApplyHumanGateCommand) error {
	return nil
}

func (*replayRuntimeActivities) CompleteRun(context.Context, workflowdomain.CompleteRunCommand) error {
	return nil
}
