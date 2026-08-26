package application

import (
	"context"
	"strings"

	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type RunView struct {
	Run   domain.WorkflowRun
	Nodes []domain.NodeRunProjection
}

type QueryRepository interface {
	GetRun(context.Context, Actor, string) (RunView, error)
}

type QueryService struct{ repository QueryRepository }

func NewQueryService(repository QueryRepository) *QueryService {
	return &QueryService{repository: repository}
}

func (service *QueryService) GetRun(ctx context.Context, actor Actor, runID string) (RunView, error) {
	runID = strings.TrimSpace(runID)
	actor.UserID = strings.TrimSpace(actor.UserID)
	if service == nil || service.repository == nil || actor.UserID == "" || actor.TokenVersion < 1 || runID == "" {
		return RunView{}, invalid("Invalid workflow query input")
	}
	view, err := service.repository.GetRun(ctx, actor, runID)
	return view, normalizeError(err)
}
