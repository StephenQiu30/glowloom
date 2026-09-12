package workflow_test

import (
	"context"
	"testing"

	workflowexecution "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/execution"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type recordingWorkflowExecutor struct {
	executors []string
}

func (executor *recordingWorkflowExecutor) Execute(
	_ context.Context,
	command workflow.NodeExecutorCommand,
) (workflow.NodeExecutorResult, error) {
	executor.executors = append(executor.executors, command.Executor)
	return workflow.NodeExecutorResult{Status: "SUCCEEDED"}, nil
}

func TestWorkflowRouterKeepsProductionWorldStoryGraphAndVisualStagesInBackendOwner(t *testing.T) {
	production := &recordingWorkflowExecutor{}
	generation := &recordingWorkflowExecutor{}
	executor, err := workflowexecution.NewNodeExecutor(production, generation)
	if err != nil {
		t.Fatal(err)
	}
	for _, executorName := range []string{
		"activity.production_world_assembly",
		"activity.production_storygraph_projection",
		"activity.project_preset_selection",
		"activity.resolve_visual_foundation",
		"activity.plan_reference_assets",
		"activity.compile_reference_briefs",
	} {
		if _, err = executor.Execute(context.Background(), workflow.NodeExecutorCommand{
			NodeActivityCommand: workflow.NodeActivityCommand{Executor: executorName},
		}); err != nil {
			t.Fatalf("route %s: %v", executorName, err)
		}
	}
	if len(production.executors) != 6 || len(generation.executors) != 0 ||
		production.executors[0] != "activity.production_world_assembly" ||
		production.executors[1] != "activity.production_storygraph_projection" ||
		production.executors[2] != "activity.project_preset_selection" ||
		production.executors[3] != "activity.resolve_visual_foundation" ||
		production.executors[4] != "activity.plan_reference_assets" ||
		production.executors[5] != "activity.compile_reference_briefs" {
		t.Fatalf("production=%v generation=%v", production.executors, generation.executors)
	}
}
