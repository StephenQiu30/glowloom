package workspace_test

import (
	"context"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	workspaceflow "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/task"
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
	next, err := workspaceflow.RetentionHandler(workspaceapp.NewProjectRetention(store))(t.Context(), taskruntime.Job{})
	if err != nil || next.IsZero() {
		t.Fatal(err)
	}
	if !store.enqueued || !store.processed {
		t.Fatal("workflow did not invoke owner service")
	}
}
