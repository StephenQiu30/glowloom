package app

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	pgmediatool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
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
	return pgworkspace.NewScopedStore(database, provideRequestScopeAccess, provideProjectWorkGuard, func(tx *gorm.DB) workspaceapp.ProjectCoverReference {
		return mediaapp.NewAssetQuery(pgmedia.NewScopedStore(tx, provideMediaScopeAccess), nil)
	})
}

func provideProjectWorkGuard(tx *gorm.DB) workspaceapp.ProjectWorkGuard {
	return projectWorkGuard{owners: []workspaceapp.ProjectWorkGuard{
		provideOperationStore(tx), pgmedia.NewScopedStore(tx, provideMediaScopeAccess), pgmediatool.NewStore(tx, nil, nil), provideMediaTranscriptionStore(tx, false),
		pgworkspace.NewProjectCopyStore(tx, nil, nil),
		provideMediaDepthStore(tx, false),
		provideScriptSourceStore(tx),
		provideMediaTransferStore(tx),
		provideMediaPurgeStore(tx),
		mediaPackageWorkGuard{store: provideMediaPackageStore(tx)},
	}}
}
