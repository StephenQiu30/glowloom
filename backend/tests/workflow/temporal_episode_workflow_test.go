package workflow_test

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	temporaladapter "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestEpisodeWorkflowExecutesCompiledOrderAndWaitsForHumanSignal(t *testing.T) {
	request := episodeWorkflowStartRequest()
	plan := temporaladapter.ExecutionPlan{
		WorkflowRunID: request.WorkflowRunID, DefinitionVersionID: request.DefinitionVersionID,
		RunInputSnapshotID: request.RunInputSnapshotID, DefinitionContentHash: request.DefinitionContentHash,
		InputSnapshotHash: request.InputSnapshotHash,
		Nodes: []temporaladapter.ExecutionNode{
			{NodeRunID: "node-run-script", NodeID: "script", Executor: "workflow.input.script_revision", RiskLevel: "low"},
			{NodeRunID: "node-run-review", NodeID: "bible-review", Executor: "gate.production_bible_review", RiskLevel: "human_gate"},
			{NodeRunID: "node-run-export", NodeID: "export", Executor: "activity.storyboard_export", RiskLevel: "low"},
		},
	}

	var suite testsuite.WorkflowTestSuite
	environment := suite.NewTestWorkflowEnvironment()
	var mu sync.Mutex
	steps := make([]string, 0, 5)
	record := func(value string) {
		mu.Lock()
		defer mu.Unlock()
		steps = append(steps, value)
	}
	environment.RegisterActivityWithOptions(
		func(context.Context, workflow.StartRequest) (temporaladapter.ExecutionPlan, error) { return plan, nil },
		activity.RegisterOptions{Name: temporaladapter.LoadExecutionPlanActivityName},
	)
	environment.RegisterActivityWithOptions(
		func(_ context.Context, command temporaladapter.NodeActivityCommand) (temporaladapter.NodeActivityResult, error) {
			record("execute:" + command.NodeID)
			return successfulNodeActivityResult(), nil
		},
		activity.RegisterOptions{Name: temporaladapter.ExecuteNodeActivityName},
	)
	environment.RegisterActivityWithOptions(
		func(_ context.Context, command temporaladapter.NodeActivityCommand) error {
			record("open:" + command.NodeID)
			return nil
		},
		activity.RegisterOptions{Name: temporaladapter.OpenHumanGateActivityName},
	)
	environment.RegisterActivityWithOptions(
		func(_ context.Context, command temporaladapter.ApplyHumanGateCommand) error {
			record("apply:" + command.NodeID)
			return nil
		},
		activity.RegisterOptions{Name: temporaladapter.ApplyHumanGateActivityName},
	)
	environment.RegisterActivityWithOptions(
		func(_ context.Context, command temporaladapter.CompleteRunCommand) error {
			record("complete:" + command.WorkflowRunID)
			return nil
		},
		activity.RegisterOptions{Name: temporaladapter.CompleteRunActivityName},
	)
	environment.RegisterDelayedCallback(func() {
		ownerReceiptID, output, outputHash := successfulHumanGateOwnerOutput()
		environment.SignalWorkflow(temporaladapter.HumanGateSignalName, temporaladapter.HumanGateSignal{
			WorkflowRunID: request.WorkflowRunID, NodeRunID: "node-run-review",
			SignalID: "signal-review", SignalIntentID: "signal-intent-review", Decision: "APPROVED",
			OwnerReceiptID: ownerReceiptID, Output: output, OutputHash: outputHash,
		})
	}, time.Minute)

	environment.ExecuteWorkflow(temporaladapter.EpisodeProductionWorkflow, request)
	if err := environment.GetWorkflowError(); err != nil {
		t.Fatalf("execute episode workflow: %v", err)
	}
	var result temporaladapter.RunResult
	if err := environment.GetWorkflowResult(&result); err != nil {
		t.Fatalf("decode workflow result: %v", err)
	}
	if result.WorkflowRunID != request.WorkflowRunID || result.Status != "SUCCEEDED" {
		t.Fatalf("workflow result = %#v", result)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{
		"execute:script", "open:bible-review", "apply:bible-review", "execute:export", "complete:" + request.WorkflowRunID,
	}
	if !slices.Equal(steps, want) {
		t.Fatalf("activity order = %v, want %v", steps, want)
	}
}

func successfulNodeActivityResult() workflow.NodeActivityResult {
	output, _, outputHash, err := workflow.BuildNodeOutput(successfulExecutorOutput())
	if err != nil {
		panic(err)
	}
	return workflow.NodeActivityResult{Status: "SUCCEEDED", Output: output, OutputHash: outputHash}
}

func successfulHumanGateOwnerOutput() (string, workflow.NodeOutputSnapshot, string) {
	output, _, outputHash, err := workflow.BuildNodeOutput(workflow.NodeOutputSnapshot{
		SchemaVersion: workflow.NodeOutputSchemaVersion,
		Bindings: []workflow.NodeOutputBinding{{
			Port: "bible", ValueType: "production_bible", ReferenceID: "00000000-0000-0000-0000-000000000333",
			ReferenceVersion: "2", ContentHash: strings.Repeat("c", 64),
		}},
	})
	if err != nil {
		panic(err)
	}
	return "00000000-0000-0000-0000-000000000334", output, outputHash
}

func approvedHumanGateSignalPreparation(intent workflow.SignalIntent) workflow.SignalPreparation {
	ownerReceiptID, output, outputHash := successfulHumanGateOwnerOutput()
	return workflow.SignalPreparation{
		ApplyReceipt: workflow.HumanGateApplyReceipt{
			ID: "00000000-0000-0000-0000-000000000335", WorkspaceID: intent.WorkspaceID,
			WorkflowRunID: intent.WorkflowRunID, NodeRunID: intent.NodeRunID, HumanTaskID: intent.HumanTaskID,
			ReviewDecisionID: intent.ReviewDecisionID, SubjectRevision: intent.SubjectRevision,
			Decision: intent.Decision, Status: "completed",
			OwnerReceiptID: ownerReceiptID, OwnerOperation: "production_bible.confirm",
			Output: output, OutputHash: outputHash,
		},
		Intent: intent,
	}
}

func TestEpisodeWorkflowRejectsDriftedExecutionPlan(t *testing.T) {
	request := episodeWorkflowStartRequest()
	plan := temporaladapter.ExecutionPlan{
		WorkflowRunID: request.WorkflowRunID, DefinitionVersionID: request.DefinitionVersionID,
		RunInputSnapshotID: request.RunInputSnapshotID, DefinitionContentHash: strings.Repeat("f", 64),
		InputSnapshotHash: request.InputSnapshotHash,
		Nodes: []temporaladapter.ExecutionNode{{
			NodeRunID: "node-run-script", NodeID: "script", Executor: "workflow.input.script_revision", RiskLevel: "low",
		}},
	}

	var suite testsuite.WorkflowTestSuite
	environment := suite.NewTestWorkflowEnvironment()
	environment.RegisterActivityWithOptions(
		func(context.Context, workflow.StartRequest) (temporaladapter.ExecutionPlan, error) { return plan, nil },
		activity.RegisterOptions{Name: temporaladapter.LoadExecutionPlanActivityName},
	)
	environment.ExecuteWorkflow(temporaladapter.EpisodeProductionWorkflow, request)
	if err := environment.GetWorkflowError(); err == nil || !strings.Contains(err.Error(), "execution plan does not match start input") {
		t.Fatalf("drifted plan error = %v", err)
	}
}

func TestEpisodeWorkflowDurablyWaitsForExternalNodeWithStableBusinessAttempt(t *testing.T) {
	request := episodeWorkflowStartRequest()
	plan := temporaladapter.ExecutionPlan{
		WorkflowRunID: request.WorkflowRunID, DefinitionVersionID: request.DefinitionVersionID,
		RunInputSnapshotID: request.RunInputSnapshotID, DefinitionContentHash: request.DefinitionContentHash,
		InputSnapshotHash: request.InputSnapshotHash,
		Nodes: []temporaladapter.ExecutionNode{{
			NodeRunID: "node-run-bible", NodeID: "bible", Executor: "activity.production_bible", RiskLevel: "external_ai",
		}},
	}

	var suite testsuite.WorkflowTestSuite
	environment := suite.NewTestWorkflowEnvironment()
	activityCalls := 0
	environment.RegisterActivityWithOptions(
		func(context.Context, workflow.StartRequest) (temporaladapter.ExecutionPlan, error) { return plan, nil },
		activity.RegisterOptions{Name: temporaladapter.LoadExecutionPlanActivityName},
	)
	environment.RegisterActivityWithOptions(
		func(_ context.Context, command temporaladapter.NodeActivityCommand) (temporaladapter.NodeActivityResult, error) {
			activityCalls++
			if command.Attempt != 1 {
				t.Fatalf("external node business attempt = %d, want stable attempt 1", command.Attempt)
			}
			if activityCalls < 3 {
				return temporaladapter.NodeActivityResult{Status: "RETRYING"}, nil
			}
			return successfulNodeActivityResult(), nil
		},
		activity.RegisterOptions{Name: temporaladapter.ExecuteNodeActivityName},
	)
	environment.RegisterActivityWithOptions(
		func(context.Context, temporaladapter.CompleteRunCommand) error { return nil },
		activity.RegisterOptions{Name: temporaladapter.CompleteRunActivityName},
	)

	environment.ExecuteWorkflow(temporaladapter.EpisodeProductionWorkflow, request)
	if err := environment.GetWorkflowError(); err != nil {
		t.Fatalf("wait for external node: %v", err)
	}
	if activityCalls != 3 {
		t.Fatalf("external node activity calls = %d, want 3", activityCalls)
	}
}

func TestEpisodeWorkflowPausesExternalNodePollingUntilResume(t *testing.T) {
	request := episodeWorkflowStartRequest()
	plan := temporaladapter.ExecutionPlan{
		WorkflowRunID: request.WorkflowRunID, DefinitionVersionID: request.DefinitionVersionID,
		RunInputSnapshotID: request.RunInputSnapshotID, DefinitionContentHash: request.DefinitionContentHash,
		InputSnapshotHash: request.InputSnapshotHash,
		Nodes: []temporaladapter.ExecutionNode{{
			NodeRunID: "node-run-bible", NodeID: "bible", Executor: "activity.production_bible", RiskLevel: "external_ai",
		}},
	}

	var suite testsuite.WorkflowTestSuite
	environment := suite.NewTestWorkflowEnvironment()
	activityCalls := 0
	polledWhilePaused := false
	environment.RegisterActivityWithOptions(
		func(context.Context, workflow.StartRequest) (temporaladapter.ExecutionPlan, error) { return plan, nil },
		activity.RegisterOptions{Name: temporaladapter.LoadExecutionPlanActivityName},
	)
	environment.RegisterActivityWithOptions(
		func(context.Context, temporaladapter.NodeActivityCommand) (temporaladapter.NodeActivityResult, error) {
			activityCalls++
			if activityCalls == 1 {
				return temporaladapter.NodeActivityResult{Status: "RETRYING"}, nil
			}
			return successfulNodeActivityResult(), nil
		},
		activity.RegisterOptions{Name: temporaladapter.ExecuteNodeActivityName},
	)
	environment.RegisterActivityWithOptions(
		func(context.Context, temporaladapter.CompleteRunCommand) error { return nil },
		activity.RegisterOptions{Name: temporaladapter.CompleteRunActivityName},
	)
	environment.RegisterDelayedCallback(func() {
		environment.SignalWorkflow(temporaladapter.WorkflowControlSignalName, temporaladapter.WorkflowControlSignal{
			WorkflowRunID: request.WorkflowRunID, ControlID: "pause-external-node", Action: workflow.ControlActionPause,
			InputHash: strings.Repeat("d", 64),
		})
	}, time.Second)
	environment.RegisterDelayedCallback(func() {
		polledWhilePaused = activityCalls > 1
		environment.SignalWorkflow(temporaladapter.WorkflowControlSignalName, temporaladapter.WorkflowControlSignal{
			WorkflowRunID: request.WorkflowRunID, ControlID: "resume-external-node", Action: workflow.ControlActionResume,
			InputHash: strings.Repeat("e", 64),
		})
	}, 10*time.Second)

	environment.ExecuteWorkflow(temporaladapter.EpisodeProductionWorkflow, request)
	if err := environment.GetWorkflowError(); err != nil {
		t.Fatalf("pause external node polling: %v", err)
	}
	if polledWhilePaused || activityCalls != 2 {
		t.Fatalf("external node calls while paused=%t total=%d", polledWhilePaused, activityCalls)
	}
}

func TestEpisodeWorkflowRejectsNodeActivityWithoutCanonicalOutput(t *testing.T) {
	request := episodeWorkflowStartRequest()
	plan := temporaladapter.ExecutionPlan{
		WorkflowRunID: request.WorkflowRunID, DefinitionVersionID: request.DefinitionVersionID,
		RunInputSnapshotID: request.RunInputSnapshotID, DefinitionContentHash: request.DefinitionContentHash,
		InputSnapshotHash: request.InputSnapshotHash,
		Nodes: []temporaladapter.ExecutionNode{{
			NodeRunID: "node-run-script", NodeID: "script", Executor: "workflow.input.script_revision", RiskLevel: "low",
		}},
	}
	var suite testsuite.WorkflowTestSuite
	environment := suite.NewTestWorkflowEnvironment()
	environment.RegisterActivityWithOptions(
		func(context.Context, workflow.StartRequest) (temporaladapter.ExecutionPlan, error) { return plan, nil },
		activity.RegisterOptions{Name: temporaladapter.LoadExecutionPlanActivityName},
	)
	environment.RegisterActivityWithOptions(
		func(context.Context, temporaladapter.NodeActivityCommand) (temporaladapter.NodeActivityResult, error) {
			return temporaladapter.NodeActivityResult{Status: "SUCCEEDED"}, nil
		},
		activity.RegisterOptions{Name: temporaladapter.ExecuteNodeActivityName},
	)
	environment.RegisterActivityWithOptions(
		func(context.Context, workflow.FailRunCommand) error { return nil },
		activity.RegisterOptions{Name: temporaladapter.FailRunActivityName},
	)
	environment.ExecuteWorkflow(temporaladapter.EpisodeProductionWorkflow, request)
	if err := environment.GetWorkflowError(); err == nil || !strings.Contains(err.Error(), "invalid terminal output") {
		t.Fatalf("invalid node activity output error = %v", err)
	}
}

func TestEpisodeWorkflowProjectsFailedNodeBeforeReturningActivityFailure(t *testing.T) {
	request := episodeWorkflowStartRequest()
	plan := temporaladapter.ExecutionPlan{
		WorkflowRunID: request.WorkflowRunID, DefinitionVersionID: request.DefinitionVersionID,
		RunInputSnapshotID: request.RunInputSnapshotID, DefinitionContentHash: request.DefinitionContentHash,
		InputSnapshotHash: request.InputSnapshotHash,
		Nodes: []temporaladapter.ExecutionNode{{
			NodeRunID: "node-run-failed", NodeID: "failed", Executor: "activity.failed", RiskLevel: "low",
		}},
	}
	var failure workflow.FailRunCommand
	var suite testsuite.WorkflowTestSuite
	environment := suite.NewTestWorkflowEnvironment()
	environment.RegisterActivityWithOptions(
		func(context.Context, workflow.StartRequest) (temporaladapter.ExecutionPlan, error) { return plan, nil },
		activity.RegisterOptions{Name: temporaladapter.LoadExecutionPlanActivityName},
	)
	environment.RegisterActivityWithOptions(
		func(context.Context, workflow.NodeActivityCommand) (temporaladapter.NodeActivityResult, error) {
			return temporaladapter.NodeActivityResult{}, temporal.NewNonRetryableApplicationError("provider failed", "provider_failed", nil)
		},
		activity.RegisterOptions{Name: temporaladapter.ExecuteNodeActivityName},
	)
	environment.RegisterActivityWithOptions(
		func(_ context.Context, command workflow.FailRunCommand) error {
			failure = command
			return nil
		},
		activity.RegisterOptions{Name: temporaladapter.FailRunActivityName},
	)
	environment.ExecuteWorkflow(temporaladapter.EpisodeProductionWorkflow, request)
	if err := environment.GetWorkflowError(); err == nil || !strings.Contains(err.Error(), "provider failed") {
		t.Fatalf("node activity failure = %v", err)
	}
	if failure.WorkflowRunID != request.WorkflowRunID || failure.NodeRunID != "node-run-failed" ||
		failure.NodeID != "failed" || failure.FailureCode != "node_activity_failed" {
		t.Fatalf("failure projection command = %#v", failure)
	}
}

func episodeWorkflowStartRequest() workflow.StartRequest {
	return workflow.StartRequest{
		WorkflowID: "lanverse:test:episode", WorkflowType: "lanverse.episode-production", WorkflowTypeVersion: "1.0.0",
		WorkflowRunID:         "00000000-0000-0000-0000-000000000111",
		DefinitionVersionID:   "00000000-0000-0000-0000-000000000222",
		RunInputSnapshotID:    "00000000-0000-0000-0000-000000000333",
		DefinitionContentHash: strings.Repeat("a", 64), InputSnapshotHash: strings.Repeat("b", 64),
		InputHash: strings.Repeat("c", 64),
	}
}
