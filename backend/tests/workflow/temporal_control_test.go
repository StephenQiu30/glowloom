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

func TestTemporalPauseResumeControlStopsAtTheNextNodeBoundaryAndReplays(t *testing.T) {
	address := os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS")
	if address == "" {
		t.Skip("set LANVERSE_TEST_TEMPORAL_ADDRESS to run the real Temporal pause/resume journey")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	temporalClient, err := client.Dial(client.Options{HostPort: address, Namespace: "default"})
	if err != nil {
		t.Fatalf("connect Temporal: %v", err)
	}
	t.Cleanup(temporalClient.Close)

	request := episodeWorkflowStartRequest()
	request.WorkflowID = "lanverse:pause-resume:" + uuid.NewString()
	request.WorkflowRunID = uuid.NewString()
	taskQueue := "lanverse-pause-resume-" + uuid.NewString()
	plan := temporaladapter.ExecutionPlan{
		WorkflowRunID: request.WorkflowRunID, DefinitionVersionID: request.DefinitionVersionID,
		RunInputSnapshotID: request.RunInputSnapshotID, DefinitionContentHash: request.DefinitionContentHash,
		InputSnapshotHash: request.InputSnapshotHash,
		Nodes: []temporaladapter.ExecutionNode{
			{NodeRunID: uuid.NewString(), NodeID: "first", Executor: "activity.first", RiskLevel: "low"},
			{NodeRunID: uuid.NewString(), NodeID: "second", Executor: "activity.second", RiskLevel: "low"},
		},
	}
	runtime, err := temporaladapter.New(temporaladapter.Config{
		Address: address, Namespace: "default", TaskQueue: taskQueue,
	})
	if err != nil {
		t.Fatalf("connect Temporal runtime: %v", err)
	}
	t.Cleanup(runtime.Close)
	activities := &pauseResumeRuntimeActivities{
		replayRuntimeActivities: &replayRuntimeActivities{plan: plan},
		firstStarted:            make(chan struct{}, 1),
		releaseFirst:            make(chan struct{}),
		secondStarted:           make(chan struct{}, 1),
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
		t.Fatalf("start pausable Episode Workflow: observation=%#v err=%v", started, err)
	}
	select {
	case <-activities.firstStarted:
	case <-ctx.Done():
		t.Fatalf("first node did not start before pause: %v", ctx.Err())
	}
	pauseIntent := workflowdomain.ControlIntent{
		ID: uuid.NewString(), TemporalWorkflowID: request.WorkflowID, ControlID: uuid.NewString(),
		WorkflowRunID: request.WorkflowRunID, Action: workflowdomain.ControlActionPause,
	}
	pauseRequest, err := workflowapp.NewControlRequest(pauseIntent)
	if err != nil {
		t.Fatalf("build pause request: %v", err)
	}
	paused, err := runtime.Control(ctx, pauseRequest)
	if err != nil || paused.Outcome != workflowdomain.ControlOutcomeApplied ||
		paused.ObservedInputHash != pauseRequest.InputHash {
		t.Fatalf("pause Episode Workflow: observation=%#v err=%v", paused, err)
	}
	replayedPause, err := runtime.Control(ctx, pauseRequest)
	if err != nil || replayedPause.Outcome != workflowdomain.ControlOutcomeAlreadyApplied ||
		replayedPause.ObservedInputHash != pauseRequest.InputHash {
		t.Fatalf("reconcile pause: observation=%#v err=%v", replayedPause, err)
	}
	driftedIntent := pauseIntent
	driftedIntent.Action = workflowdomain.ControlActionResume
	driftedRequest, err := workflowapp.NewControlRequest(driftedIntent)
	if err != nil {
		t.Fatalf("build drifted pause request: %v", err)
	}
	conflict, err := runtime.Control(ctx, driftedRequest)
	if err != nil || conflict.Outcome != workflowdomain.ControlOutcomeConflict ||
		conflict.ObservedInputHash != pauseRequest.InputHash {
		t.Fatalf("reject drifted pause request: observation=%#v err=%v", conflict, err)
	}

	close(activities.releaseFirst)
	select {
	case <-activities.secondStarted:
		t.Fatal("second node started while the Episode Workflow was paused")
	case <-time.After(500 * time.Millisecond):
	}
	resumeIntent := workflowdomain.ControlIntent{
		ID: uuid.NewString(), TemporalWorkflowID: request.WorkflowID, ControlID: uuid.NewString(),
		WorkflowRunID: request.WorkflowRunID, Action: workflowdomain.ControlActionResume,
	}
	resumeRequest, err := workflowapp.NewControlRequest(resumeIntent)
	if err != nil {
		t.Fatalf("build resume request: %v", err)
	}
	resumed, err := runtime.Control(ctx, resumeRequest)
	if err != nil || resumed.Outcome != workflowdomain.ControlOutcomeApplied ||
		resumed.ObservedInputHash != resumeRequest.InputHash {
		t.Fatalf("resume Episode Workflow: observation=%#v err=%v", resumed, err)
	}
	select {
	case <-activities.secondStarted:
	case <-ctx.Done():
		t.Fatalf("second node did not start after resume: %v", ctx.Err())
	}
	if err = temporalClient.GetWorkflow(ctx, request.WorkflowID, "").Get(ctx, nil); err != nil {
		t.Fatalf("wait resumed Episode Workflow: %v", err)
	}

	iterator := temporalClient.GetWorkflowHistory(
		ctx, request.WorkflowID, "", false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT,
	)
	history := &historypb.History{}
	controlSignals := 0
	for iterator.HasNext() {
		event, nextErr := iterator.Next()
		if nextErr != nil {
			t.Fatalf("read pause/resume workflow history: %v", nextErr)
		}
		history.Events = append(history.Events, event)
		if attributes := event.GetWorkflowExecutionSignaledEventAttributes(); attributes != nil &&
			attributes.GetSignalName() == temporaladapter.WorkflowControlSignalName {
			controlSignals++
		}
	}
	if controlSignals != 2 {
		t.Fatalf("Temporal pause/resume signal events = %d, want 2", controlSignals)
	}
	replayer := worker.NewWorkflowReplayer()
	replayer.RegisterWorkflowWithOptions(
		temporaladapter.EpisodeProductionWorkflow,
		temporalworkflow.RegisterOptions{Name: temporaladapter.EpisodeProductionWorkflowName},
	)
	if err = replayer.ReplayWorkflowHistory(nil, history); err != nil {
		t.Fatalf("replay paused/resumed Episode Workflow history: %v", err)
	}
}

type controlRuntimeActivities struct {
	*replayRuntimeActivities
	humanGateOpened chan struct{}
}

type pauseResumeRuntimeActivities struct {
	*replayRuntimeActivities
	firstStarted  chan struct{}
	releaseFirst  chan struct{}
	secondStarted chan struct{}
}

func (activities *pauseResumeRuntimeActivities) ExecuteNode(
	ctx context.Context,
	command workflowdomain.NodeActivityCommand,
) (workflowdomain.NodeActivityResult, error) {
	switch command.NodeID {
	case "first":
		activities.firstStarted <- struct{}{}
		select {
		case <-activities.releaseFirst:
		case <-ctx.Done():
			return workflowdomain.NodeActivityResult{}, ctx.Err()
		}
	case "second":
		activities.secondStarted <- struct{}{}
	}
	return successfulNodeActivityResult(), nil
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
