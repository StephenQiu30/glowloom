package review

import (
	"context"

	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type Opener struct {
	service *reviewapp.Service
}

func New(service *reviewapp.Service) *Opener {
	return &Opener{service: service}
}

func (opener *Opener) OpenHumanTask(ctx context.Context, binding domain.HumanGateBinding) error {
	if opener == nil || opener.service == nil {
		return reviewapp.ErrNotFound
	}
	_, err := opener.service.Open(ctx, reviewapp.OpenCommand{
		WorkspaceID: binding.WorkspaceID, ProjectID: binding.ProjectID,
		WorkflowRunID: binding.WorkflowRunID, NodeRunID: binding.NodeRunID,
		SubjectType: binding.SubjectType, SubjectID: binding.SubjectID, SubjectRevision: binding.SubjectRevision,
		CandidateIDs: binding.CandidateIDs, RubricVersion: binding.RubricVersion,
	})
	return err
}

var _ workflowapp.HumanTaskOpener = (*Opener)(nil)
