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
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"

	temporaladapter "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflowdomain "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestTemporalCancelControlReconcilesOneRequestAndReplaysHistory(t *testing.T) {
	address := os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS")
	if address == "" {
		t.Skip("set LANVERSE_TEST_TEMPORAL_ADDRESS to run the real Temporal control journey")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	temporalClient, err := client.Dial(client.Options{HostPort: address, Namespace: "default"})
	if err != nil {
		t.Fatalf("connect Temporal: %v", err)
	}
	t.Cleanup(temporalClient.Close)

	request := episodeWorkflowStartRequest()
	request.WorkflowID = "lanverse:control:" + uuid.NewString()
	request.WorkflowRunID = uuid.NewString()
	taskQueue := "lanverse-control-" + uuid.NewString()
	plan := temporaladapter.ExecutionPlan{
		WorkflowRunID: request.WorkflowRunID, DefinitionVersionID: request.DefinitionVersionID,
		RunInputSnapshotID: request.RunInputSnapshotID, DefinitionContentHash: request.DefinitionContentHash,
		InputSnapshotHash: request.InputSnapshotHash,
		Nodes: []temporaladapter.ExecutionNode{{
			NodeRunID: uuid.NewString(), NodeID: "review",
			Executor: "gate.production_bible_review", RiskLevel: "human_gate",
		}},
	}
	runtime, err := temporaladapter.New(temporaladapter.Config{
		Address: address, Namespace: "default", TaskQueue: taskQueue,
	})
	if err != nil {
		t.Fatalf("connect Temporal runtime: %v", err)
	}
	t.Cleanup(runtime.Close)
	activities := &controlRuntimeActivities{
		replayRuntimeActivities: &replayRuntimeActivities{plan: plan},
		humanGateOpened:         make(chan struct{}, 1),
	}
	runtimeWorker, err := runtime.NewWorker(activities)
	if err != nil {
		t.Fatalf("register Temporal worker: %v", err)
	}
	if err = runtimeWorker.Start(); err != nil {
		t.Fatalf("start Temporal worker: %v", err)
	}
	t.Cleanup(runtimeWorker.Stop)

	started, err := runtime.Start(ctx, request)
	if err != nil || started.Outcome != workflowdomain.StartOutcomeStarted {
		t.Fatalf("start cancellable Episode Workflow: observation=%#v err=%v", started, err)
	}
	select {
	case <-activities.humanGateOpened:
	case <-ctx.Done():
		t.Fatalf("Episode Workflow did not reach Human Gate before cancellation: %v", ctx.Err())
	}
	intent := workflowdomain.ControlIntent{
		ID: uuid.NewString(), TemporalWorkflowID: request.WorkflowID, ControlID: uuid.NewString(),
		WorkflowRunID: request.WorkflowRunID, Action: workflowdomain.ControlActionCancel,
	}
	controlRequest, err := workflowapp.NewControlRequest(intent)
	if err != nil {
		t.Fatalf("build cancel control request: %v", err)
	}
	applied, err := runtime.Control(ctx, controlRequest)
	if err != nil || applied.Outcome != workflowdomain.ControlOutcomeApplied ||
		applied.ObservedInputHash != controlRequest.InputHash {
		t.Fatalf("cancel real Temporal workflow: observation=%#v err=%v", applied, err)
	}
	alreadyApplied, err := runtime.Control(ctx, controlRequest)
	if err != nil || alreadyApplied.Outcome != workflowdomain.ControlOutcomeAlreadyApplied ||
		alreadyApplied.ObservedInputHash != controlRequest.InputHash {
		t.Fatalf("reconcile repeated cancellation: observation=%#v err=%v", alreadyApplied, err)
	}
	driftedIntent := intent
	driftedIntent.WorkflowRunID = uuid.NewString()
	driftedRequest, err := workflowapp.NewControlRequest(driftedIntent)
	if err != nil {
		t.Fatalf("build drifted cancel request: %v", err)
	}
	conflict, err := runtime.Control(ctx, driftedRequest)
	if err != nil || conflict.Outcome != workflowdomain.ControlOutcomeConflict ||
		conflict.ObservedInputHash != controlRequest.InputHash {
		t.Fatalf("reject drifted repeated cancellation: observation=%#v err=%v", conflict, err)
	}

	run := temporalClient.GetWorkflow(ctx, request.WorkflowID, "")
	if err = run.Get(ctx, nil); err == nil || !temporal.IsCanceledError(err) {
		t.Fatalf("Temporal workflow terminal error = %v, want canceled", err)
	}
	iterator := temporalClient.GetWorkflowHistory(
		ctx, request.WorkflowID, "", false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT,
	)
	history := &historypb.History{}
	cancelRequests := 0
	for iterator.HasNext() {
		event, nextErr := iterator.Next()
		if nextErr != nil {
			t.Fatalf("read workflow history: %v", nextErr)
		}
		history.Events = append(history.Events, event)
		if event.GetWorkflowExecutionCancelRequestedEventAttributes() != nil {
			cancelRequests++
		}
	}
	if cancelRequests != 1 {
		t.Fatalf("Temporal cancel request events = %d, want 1", cancelRequests)
	}
	replayer := worker.NewWorkflowReplayer()
	replayer.RegisterWorkflowWithOptions(
		temporaladapter.EpisodeProductionWorkflow,
		temporalworkflow.RegisterOptions{Name: temporaladapter.EpisodeProductionWorkflowName},
	)
	if err = replayer.ReplayWorkflowHistory(nil, history); err != nil {
		t.Fatalf("replay cancelled Episode Workflow history: %v", err)
	}
}

type controlRuntimeActivities struct {
	*replayRuntimeActivities
	humanGateOpened chan struct{}
}

func (activities *controlRuntimeActivities) OpenHumanGate(
	context.Context,
	workflowdomain.NodeActivityCommand,
) error {
	select {
	case activities.humanGateOpened <- struct{}{}:
	default:
	}
	return nil
}
