package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/stephenqiu30/lanverse/backend/internal/modules/scripts/adapters/postgres"
	"github.com/stephenqiu30/lanverse/backend/internal/modules/scripts/domain"
)

type Service struct {
	repository *postgres.Repository
}

func NewService(repository *postgres.Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) CreateWorkspace(ctx context.Context, name string) (domain.Workspace, error) {
	if name == "" {
		return domain.Workspace{}, fmt.Errorf("workspace name is required")
	}
	return s.repository.CreateWorkspace(ctx, name)
}

func (s *Service) CreateProject(ctx context.Context, workspaceID uuid.UUID, name string) (domain.Project, error) {
	if name == "" {
		return domain.Project{}, fmt.Errorf("project name is required")
	}
	return s.repository.CreateProject(ctx, workspaceID, name)
}

func (s *Service) CreateScriptRevision(ctx context.Context, projectID uuid.UUID, name, content string) (domain.ScriptRevision, error) {
	if name == "" {
		return domain.ScriptRevision{}, fmt.Errorf("script name is required")
	}
	return s.repository.CreateScriptRevision(ctx, projectID, name, content)
}

func (s *Service) QueueAnalysis(ctx context.Context, revisionID uuid.UUID) (domain.Operation, error) {
	return s.repository.QueueAnalysis(ctx, revisionID)
}

func (s *Service) GetOperation(ctx context.Context, operationID uuid.UUID) (domain.Operation, error) {
	return s.repository.GetOperation(ctx, operationID)
}

func (s *Service) ApproveAnalysis(ctx context.Context, revisionID uuid.UUID) (domain.Analysis, error) {
	return s.repository.ApproveAnalysis(ctx, revisionID)
}

func (s *Service) GetProjectAnalysis(ctx context.Context, projectID uuid.UUID) (domain.Analysis, error) {
	return s.repository.GetProjectAnalysis(ctx, projectID)
}

func (s *Service) GetAnalysisDraft(ctx context.Context, revisionID uuid.UUID) (domain.Analysis, error) {
	return s.repository.GetAnalysisDraft(ctx, revisionID)
}
