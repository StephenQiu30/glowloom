package application

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
)

var ErrVisualFoundationSourceUnavailable = errors.New("confirmed Visual Foundation source unavailable")

type VisualFoundationSourceRepository interface {
	GetCurrentVisualFoundationSource(context.Context, string, string) (domain.ConfirmedVisualFoundationSource, error)
}

type VisualFoundationSourceService struct {
	repository VisualFoundationSourceRepository
}

func NewVisualFoundationSourceService(repository VisualFoundationSourceRepository) (*VisualFoundationSourceService, error) {
	if repository == nil {
		return nil, errors.New("Visual Foundation source repository is unavailable")
	}
	return &VisualFoundationSourceService{repository: repository}, nil
}

func (service *VisualFoundationSourceService) Current(
	ctx context.Context,
	workspaceID, projectID string,
) (domain.ConfirmedVisualFoundationSource, error) {
	if service == nil || service.repository == nil {
		return domain.ConfirmedVisualFoundationSource{}, ErrVisualFoundationSourceUnavailable
	}
	workspace, workspaceErr := uuid.Parse(workspaceID)
	project, projectErr := uuid.Parse(projectID)
	if workspaceErr != nil || projectErr != nil || workspace == uuid.Nil || project == uuid.Nil {
		return domain.ConfirmedVisualFoundationSource{}, ErrVisualFoundationSourceUnavailable
	}
	value, err := service.repository.GetCurrentVisualFoundationSource(ctx, workspaceID, projectID)
	if err != nil {
		return domain.ConfirmedVisualFoundationSource{}, err
	}
	if value.WorkspaceID != workspaceID || value.ProjectID != projectID || value.Validate() != nil {
		return domain.ConfirmedVisualFoundationSource{}, ErrVisualFoundationSourceUnavailable
	}
	return value, nil
}
