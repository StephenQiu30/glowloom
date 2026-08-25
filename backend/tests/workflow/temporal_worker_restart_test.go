package workflow_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/google/uuid"
	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"

	temporaladapter "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflowdomain "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const (
	temporalWorkerHelperFlag      = "LANVERSE_TEMPORAL_WORKER_HELPER"
	temporalWorkerHelperAddress   = "LANVERSE_TEMPORAL_WORKER_ADDRESS"
	temporalWorkerHelperTaskQueue = "LANVERSE_TEMPORAL_WORKER_TASK_QUEUE"
	temporalWorkerHelperPlan      = "LANVERSE_TEMPORAL_WORKER_PLAN"
)

func TestTemporalWorkerRecoversHumanWaitAfterCrossProcessRestart(t *testing.T) {
	address := os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS")
	if address == "" {
		t.Skip("set LANVERSE_TEST_TEMPORAL_ADDRESS to run the real Temporal worker recovery journey")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	temporalClient, err := client.Dial(client.Options{HostPort: address, Namespace: "default"})
	if err != nil {
		t.Fatalf("connect Temporal: %v", err)
	}
	t.Cleanup(temporalClient.Close)

	request := episodeWorkflowStartRequest()
	request.WorkflowID = "lanverse:worker-restart:" + uuid.NewString()
	request.WorkflowRunID = uuid.NewString()
	taskQueue := "lanverse-worker-restart-" + uuid.NewString()
	plan := temporaladapter.ExecutionPlan{
		WorkflowRunID: request.WorkflowRunID, DefinitionVersionID: request.DefinitionVersionID,
		RunInputSnapshotID: request.RunInputSnapshotID, DefinitionContentHash: request.DefinitionContentHash,
		InputSnapshotHash: request.InputSnapshotHash,
		Nodes: []temporaladapter.ExecutionNode{
			{NodeRunID: uuid.NewString(), NodeID: "prepare", Executor: "activity.prepare", RiskLevel: "low"},
			{NodeRunID: uuid.NewString(), NodeID: "review", Executor: "gate.review", RiskLevel: "human_gate"},
			{NodeRunID: uuid.NewString(), NodeID: "export", Executor: "activity.export", RiskLevel: "low"},
		},
	}

	firstWorker, firstOutput := startTemporalWorkerProcess(t, address, taskQueue, plan)
	runtime, err := temporaladapter.New(temporaladapter.Config{
		Address: address, Namespace: "default", TaskQueue: taskQueue,
	})
	if err != nil {
		t.Fatalf("connect Temporal runtime: %v", err)
	}
	t.Cleanup(runtime.Close)
	started, err := runtime.Start(ctx, request)
	if err != nil || started.Outcome != workflowdomain.StartOutcomeStarted {
		t.Fatalf("start Episode Workflow: observation=%#v err=%v", started, err)
	}
	waitForCompletedActivity(
		t, ctx, temporalClient, request.WorkflowID, "open-human-gate:"+plan.Nodes[1].NodeRunID,
	)
	stopWorkflowWorkerProcess(t, firstWorker, firstOutput)

	signalIntent := workflowdomain.SignalIntent{
		ID: uuid.NewString(), TemporalWorkflowID: request.WorkflowID, SignalID: uuid.NewString(),
		WorkflowRunID: request.WorkflowRunID, NodeRunID: plan.Nodes[1].NodeRunID, Decision: "approved",
	}
	signalRequest, err := workflowapp.NewSignalRequest(signalIntent)
	if err != nil {
		t.Fatalf("build Human Gate signal: %v", err)
	}
	signaled, err := runtime.Signal(ctx, signalRequest)
	if signaled.Outcome == workflowdomain.SignalOutcomeSignaled {
		if err != nil || signaled.ObservedInputHash != signalRequest.InputHash {
			t.Fatalf("record Human Gate signal while no worker is running: observation=%#v err=%v", signaled, err)
		}
	} else if signaled.Outcome != workflowdomain.SignalOutcomeUnknown || err == nil {
		t.Fatalf("record or preserve an unknown Human Gate signal outcome: observation=%#v err=%v", signaled, err)
	}
	waitForHumanGateSignal(t, ctx, temporalClient, request.WorkflowID, signalRequest.SignalID)
	reconciled, err := runtime.Signal(ctx, signalRequest)
	if err != nil || reconciled.Outcome != workflowdomain.SignalOutcomeAlreadyApplied ||
		reconciled.ObservedInputHash != signalRequest.InputHash {
		t.Fatalf("reconcile Human Gate signal with the same identity: observation=%#v err=%v", reconciled, err)
	}

	secondWorker, secondOutput := startTemporalWorkerProcess(t, address, taskQueue, plan)
	var result temporaladapter.RunResult
	if err = temporalClient.GetWorkflow(ctx, request.WorkflowID, "").Get(ctx, &result); err != nil {
		t.Fatalf("wait recovered Episode Workflow: %v", err)
	}
	if result.WorkflowRunID != request.WorkflowRunID || result.Status != "SUCCEEDED" {
		t.Fatalf("recovered workflow result = %#v", result)
	}
	stopWorkflowWorkerProcess(t, secondWorker, secondOutput)

	history, humanGateSignals, activityStarts, activityCompletions := loadRecoveredWorkflowHistory(
		t, ctx, temporalClient, request.WorkflowID,
	)
	if humanGateSignals != 1 {
		t.Fatalf("Temporal Human Gate signal events = %d, want 1", humanGateSignals)
	}
	wantActivities := []string{
		"load-execution-plan",
		"execute-node:" + plan.Nodes[0].NodeRunID,
		"open-human-gate:" + plan.Nodes[1].NodeRunID,
		"apply-human-gate:" + plan.Nodes[1].NodeRunID,
		"execute-node:" + plan.Nodes[2].NodeRunID,
		"complete-run",
	}
	if len(activityStarts) != len(wantActivities) || len(activityCompletions) != len(wantActivities) {
		t.Fatalf("Temporal activity sets: starts=%#v completions=%#v", activityStarts, activityCompletions)
	}
	for _, activityID := range wantActivities {
		if activityStarts[activityID] != 1 || activityCompletions[activityID] != 1 {
			t.Fatalf(
				"Temporal activity %q counts: starts=%d completions=%d, want 1/1",
				activityID, activityStarts[activityID], activityCompletions[activityID],
			)
		}
	}

	replayer := worker.NewWorkflowReplayer()
	replayer.RegisterWorkflowWithOptions(
		temporaladapter.EpisodeProductionWorkflow,
		temporalworkflow.RegisterOptions{Name: temporaladapter.EpisodeProductionWorkflowName},
	)
	if err = replayer.ReplayWorkflowHistory(nil, history); err != nil {
		t.Fatalf("replay recovered Episode Workflow history: %v", err)
	}
}

func TestTemporalWorkerProcessHelper(t *testing.T) {
	if os.Getenv(temporalWorkerHelperFlag) != "1" {
		t.Skip("subprocess helper")
	}
	planPayload, err := base64.RawStdEncoding.DecodeString(os.Getenv(temporalWorkerHelperPlan))
	if err != nil {
		t.Fatalf("decode worker plan: %v", err)
	}
	var plan temporaladapter.ExecutionPlan
	if err = json.Unmarshal(planPayload, &plan); err != nil {
		t.Fatalf("parse worker plan: %v", err)
	}
	runtime, err := temporaladapter.New(temporaladapter.Config{
		Address: os.Getenv(temporalWorkerHelperAddress), Namespace: "default",
		TaskQueue: os.Getenv(temporalWorkerHelperTaskQueue),
	})
	if err != nil {
		t.Fatalf("connect helper Temporal runtime: %v", err)
	}
	runtimeWorker, err := runtime.NewWorker(&replayRuntimeActivities{plan: plan})
	if err != nil {
		t.Fatalf("create helper Temporal worker: %v", err)
	}
	if err = runtimeWorker.Start(); err != nil {
		t.Fatalf("start helper Temporal worker: %v", err)
	}
	select {}
}

func startTemporalWorkerProcess(
	t *testing.T,
	address string,
	taskQueue string,
	plan temporaladapter.ExecutionPlan,
) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	planPayload, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("encode worker plan: %v", err)
	}
	output := &bytes.Buffer{}
	command := exec.Command(os.Args[0], "-test.run=^TestTemporalWorkerProcessHelper$", "-test.v")
	command.Env = append(os.Environ(),
		temporalWorkerHelperFlag+"=1",
		temporalWorkerHelperAddress+"="+address,
		temporalWorkerHelperTaskQueue+"="+taskQueue,
		temporalWorkerHelperPlan+"="+base64.RawStdEncoding.EncodeToString(planPayload),
	)
	command.Stdout, command.Stderr = output, output
	if err = command.Start(); err != nil {
		t.Fatalf("start Temporal worker subprocess: %v", err)
	}
	t.Cleanup(func() {
		if command.ProcessState == nil && command.Process != nil {
			_ = command.Process.Kill()
			_, _ = command.Process.Wait()
		}
	})
	return command, output
}

func waitForCompletedActivity(
	t *testing.T,
	ctx context.Context,
	temporalClient client.Client,
	workflowID string,
	activityID string,
) {
	t.Helper()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		iterator := temporalClient.GetWorkflowHistory(
			ctx, workflowID, "", false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT,
		)
		scheduled := make(map[int64]string)
		for iterator.HasNext() {
			event, err := iterator.Next()
			if err != nil {
				t.Fatalf("read workflow history while waiting for activity completion: %v", err)
			}
			if attributes := event.GetActivityTaskScheduledEventAttributes(); attributes != nil {
				scheduled[event.GetEventId()] = attributes.GetActivityId()
			}
			if attributes := event.GetActivityTaskCompletedEventAttributes(); attributes != nil &&
				scheduled[attributes.GetScheduledEventId()] == activityID {
				return
			}
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("wait for Temporal activity %q completion: %v", activityID, ctx.Err())
		}
	}
}

func waitForHumanGateSignal(
	t *testing.T,
	ctx context.Context,
	temporalClient client.Client,
	workflowID string,
	signalID string,
) {
	t.Helper()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		iterator := temporalClient.GetWorkflowHistory(
			ctx, workflowID, "", false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT,
		)
		for iterator.HasNext() {
			event, err := iterator.Next()
			if err != nil {
				t.Fatalf("read workflow history while waiting for Human Gate signal: %v", err)
			}
			if attributes := event.GetWorkflowExecutionSignaledEventAttributes(); attributes != nil &&
				attributes.GetSignalName() == temporaladapter.HumanGateSignalName {
				var signal temporaladapter.HumanGateSignal
				if err = converter.GetDefaultDataConverter().FromPayloads(attributes.GetInput(), &signal); err != nil {
					t.Fatalf("decode Temporal Human Gate signal: %v", err)
				}
				if signal.SignalID == signalID {
					return
				}
			}
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("wait for Temporal Human Gate signal %q: %v", signalID, ctx.Err())
		}
	}
}

func loadRecoveredWorkflowHistory(
	t *testing.T,
	ctx context.Context,
	temporalClient client.Client,
	workflowID string,
) (*historypb.History, int, map[string]int, map[string]int) {
	t.Helper()
	iterator := temporalClient.GetWorkflowHistory(
		ctx, workflowID, "", false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT,
	)
	history := &historypb.History{}
	humanGateSignals := 0
	scheduled := make(map[int64]string)
	activityStarts := make(map[string]int)
	activityCompletions := make(map[string]int)
	for iterator.HasNext() {
		event, err := iterator.Next()
		if err != nil {
			t.Fatalf("read recovered workflow history: %v", err)
		}
		history.Events = append(history.Events, event)
		if attributes := event.GetActivityTaskScheduledEventAttributes(); attributes != nil {
			scheduled[event.GetEventId()] = attributes.GetActivityId()
		}
		if attributes := event.GetActivityTaskStartedEventAttributes(); attributes != nil {
			activityStarts[scheduled[attributes.GetScheduledEventId()]]++
		}
		if attributes := event.GetActivityTaskCompletedEventAttributes(); attributes != nil {
			activityCompletions[scheduled[attributes.GetScheduledEventId()]]++
		}
		if attributes := event.GetWorkflowExecutionSignaledEventAttributes(); attributes != nil &&
			attributes.GetSignalName() == temporaladapter.HumanGateSignalName {
			humanGateSignals++
		}
	}
	return history, humanGateSignals, activityStarts, activityCompletions
}
