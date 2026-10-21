package workspace_test

import (
	"context"
	"testing"

	"go.temporal.io/sdk/testsuite"

	workspaceflow "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/workflow"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

type retentionWorkflowStore struct{ enqueued, processed bool }

func (s *retentionWorkflowStore) EnqueueExpired(context.Context, int) (int64, error) {
	s.enqueued = true
	return 1, nil
}
func (s *retentionWorkflowStore) ProcessNext(context.Context) (bool, error) {
	if s.processed {
		return false, nil
	}
	s.processed = true
	return true, nil
}
func TestProjectRetentionWorkflowInvokesBoundedOwnerService(t *testing.T) {
	store := &retentionWorkflowStore{}
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterActivity(workspaceflow.NewProjectRetentionActivities(workspaceapp.NewProjectRetention(store)))
	env.ExecuteWorkflow(workspaceflow.ProjectRetentionWorkflow)
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	if !store.enqueued || !store.processed {
		t.Fatal("workflow did not invoke owner service")
	}
}
