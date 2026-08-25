package bootstrap

import (
	"errors"
	"time"

	"github.com/google/uuid"

	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	workflowreview "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/review"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
)

type WorkflowRuntimeRepository interface {
	workflowapp.RuntimeRepository
	workflowapp.NodeRuntimeRepository
	workflowapp.NodeCacheRuntimeRepository
	workflowapp.RunCompletionRepository
	workflowapp.HumanGateRepository
	workflowapp.HumanGateApplyRepository
}

func NewWorkflowRuntime(
	repository WorkflowRuntimeRepository,
	scripts workflowproduction.ScriptSource,
	reviews *reviewapp.Service,
) (*workflowapp.RuntimeService, error) {
	if repository == nil || scripts == nil || reviews == nil {
		return nil, errors.New("workflow runtime dependencies are required")
	}
	now := func() time.Time { return time.Now().UTC() }
	return workflowapp.NewRuntimeService(repository, workflowapp.RuntimeConfig{
		Now: now, NewID: uuid.NewString,
		Executor:   workflowproduction.NewNodeExecutor(scripts),
		HumanTasks: workflowreview.New(reviews),
	}), nil
}
