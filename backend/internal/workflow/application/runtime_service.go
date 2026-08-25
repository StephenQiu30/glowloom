package application

import (
	"context"
	"strings"

	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type RuntimeRepository interface {
	LoadExecutionPlan(context.Context, domain.StartRequest) (domain.ExecutionPlan, error)
}

type RuntimeService struct {
	repository RuntimeRepository
}

func NewRuntimeService(repository RuntimeRepository) *RuntimeService {
	return &RuntimeService{repository: repository}
}

func (service *RuntimeService) LoadExecutionPlan(ctx context.Context, request domain.StartRequest) (domain.ExecutionPlan, error) {
	if service == nil || service.repository == nil || strings.TrimSpace(request.WorkflowRunID) == "" ||
		strings.TrimSpace(request.DefinitionVersionID) == "" || strings.TrimSpace(request.RunInputSnapshotID) == "" ||
		len(request.DefinitionContentHash) != 64 || len(request.InputSnapshotHash) != 64 || len(request.InputHash) != 64 {
		return domain.ExecutionPlan{}, invalid("Invalid workflow runtime input")
	}
	plan, err := service.repository.LoadExecutionPlan(ctx, request)
	return plan, normalizeError(err)
}
