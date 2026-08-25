package workflow_test

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"go.temporal.io/sdk/activity"
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
			return temporaladapter.NodeActivityResult{Status: "SUCCEEDED"}, nil
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
		environment.SignalWorkflow(temporaladapter.HumanGateSignalName, temporaladapter.HumanGateSignal{
			WorkflowRunID: request.WorkflowRunID, NodeRunID: "node-run-review",
			SignalIntentID: "signal-intent-review", Decision: "APPROVED",
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
