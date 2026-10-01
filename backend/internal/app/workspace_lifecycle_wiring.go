package app

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	pgmediatool "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/postgres"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

type projectWorkGuard struct {
	owners []workspaceapp.ProjectWorkGuard
}

func (g projectWorkGuard) HasInflightWork(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID) (bool, error) {
	blocked := false
	// These queries share one SQL transaction/connection and must be sequential.
	// Every installed owner is checked; an unreadable owner is never assumed idle.
	for _, owner := range g.owners {
		if owner == nil {
			return false, workspaceapp.ErrProjectDependencyUnavailable
		}
		active, err := owner.HasInflightWork(ctx, actor, projectID)
		if err != nil {
			return false, fmt.Errorf("read lifecycle owner: %w", err)
		}
		blocked = blocked || active
	}
	return blocked, nil
}

func provideWorkspaceLifecycleStore(database *gorm.DB) *pgworkspace.Store {
	return pgworkspace.NewStoreWithProjectWorkGuard(database, func(tx *gorm.DB) workspaceapp.ProjectWorkGuard {
		return projectWorkGuard{owners: []workspaceapp.ProjectWorkGuard{pgoperation.NewStore(tx), pgmedia.NewStore(tx), pgmediatool.NewStore(tx, nil, nil), provideMediaTranscriptionStore(tx, false)}}
	})
}
