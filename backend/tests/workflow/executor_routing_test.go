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

func TestWorkflowRouterKeepsProductionWorldAndStoryGraphInBackendOwner(t *testing.T) {
	production := &recordingWorkflowExecutor{}
	generation := &recordingWorkflowExecutor{}
	executor, err := workflowexecution.NewNodeExecutor(production, generation)
	if err != nil {
		t.Fatal(err)
	}
	for _, executorName := range []string{
		"activity.production_world_assembly",
		"activity.production_storygraph_projection",
	} {
		if _, err = executor.Execute(context.Background(), workflow.NodeExecutorCommand{
			NodeActivityCommand: workflow.NodeActivityCommand{Executor: executorName},
		}); err != nil {
			t.Fatalf("route %s: %v", executorName, err)
		}
	}
	if len(production.executors) != 2 || len(generation.executors) != 0 ||
		production.executors[0] != "activity.production_world_assembly" ||
		production.executors[1] != "activity.production_storygraph_projection" {
		t.Fatalf("production=%v generation=%v", production.executors, generation.executors)
	}
}
