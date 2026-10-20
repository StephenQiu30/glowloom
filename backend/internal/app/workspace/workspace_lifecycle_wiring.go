package workspace

import (
	"context"
	"fmt"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"

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

// ProvideWorkspaceLifecycleStore assembles workspace lifecycle storage.
func ProvideWorkspaceLifecycleStore(database *gorm.DB) *pgworkspace.Store {
	return pgworkspace.NewScopedStore(database, scopewire.ProvideRequestScopeAccess, ProvideProjectWorkGuard, func(tx *gorm.DB) workspaceapp.ProjectCoverReference {
		return mediaapp.NewAssetQuery(pgmedia.NewScopedStore(tx, mediawire.NewMediaScopeAccess), nil)
	})
}

// ProvideProjectWorkGuard assembles project lifecycle work guards.
func ProvideProjectWorkGuard(tx *gorm.DB) workspaceapp.ProjectWorkGuard {
	return projectWorkGuard{owners: []workspaceapp.ProjectWorkGuard{
		operationwire.ProvideOperationStore(tx), pgmedia.NewScopedStore(tx, mediawire.NewMediaScopeAccess), pgmediatool.NewStore(tx, nil, nil), mediawire.ProvideMediaTranscriptionStore(tx, false),
		pgworkspace.NewProjectCopyStore(tx, nil, nil),
		mediawire.ProvideMediaDepthStore(tx, false),
		scriptwire.ProvideScriptSourceStore(tx),
		mediawire.ProvideMediaTransferStore(tx),
		mediawire.ProvideMediaPurgeStore(tx),
		mediawire.PackageWorkGuard(tx),
	}}
}
