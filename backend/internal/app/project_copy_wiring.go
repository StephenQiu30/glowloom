package app

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	pgcanvas "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	workspacedomain "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// copiedCanvasMedia binds private target references to one frozen copying job.
// Ordinary media queries continue to reject unpublished target projects.
type copiedCanvasMedia struct {
	store   *pgmedia.ProjectCopyStore
	binding mediaapp.ProjectCopyBinding
}

func (r copiedCanvasMedia) Reference(ctx context.Context, actor identityapp.Principal, projectID, assetID uuid.UUID) (mediaapp.AssetSummary, error) {
	if projectID != r.binding.TargetProjectID {
		return mediaapp.AssetSummary{}, mediaapp.ErrNotFound
	}
	return r.store.ReferenceCopiedAsset(ctx, actor, r.binding, assetID)
}

func provideProjectCopyStore(database *gorm.DB) *pgworkspace.ProjectCopyStore {
	return pgworkspace.NewProjectCopyStoreWithScript(database, provideProjectWorkGuard,
		func(tx *gorm.DB) workspaceapp.ProjectCopyOwners {
			media := pgmedia.NewProjectCopyStore(tx)
			canvas := pgcanvas.NewProjectCopyStore(tx,
				func(mediaTx *gorm.DB) canvasapp.MediaReader {
					return mediaapp.NewAssetQuery(pgmedia.NewStore(mediaTx), nil)
				},
				func(mediaTx *gorm.DB, binding canvasapp.ProjectCopyBinding) canvasapp.MediaReader {
					return copiedCanvasMedia{store: pgmedia.NewProjectCopyStore(mediaTx), binding: mediaapp.ProjectCopyBinding{
						JobID: binding.JobID, OrgID: binding.OrgID, SourceProjectID: binding.SourceProjectID, TargetProjectID: binding.TargetProjectID,
					}}
				})
			return workspaceapp.ProjectCopyOwners{Media: media, Canvas: canvas, Budget: pgbilling.NewStore(tx), Cover: projectCopyCoverOwner{tx: tx}}
		}, func(tx *gorm.DB, authority workspaceapp.ProjectCopyAuthority) workspaceapp.ProjectCopyScriptOwner {
			return projectCopyScriptOwner{store: provideScriptProjectCopyStore(tx, authority)}
		})
}

func provideProjectCopyWorker(database *gorm.DB, storage *objectstorage.Client) (*workspaceapp.ProjectCopyWorker, *pgworkspace.ProjectCopyStore) {
	store := provideProjectCopyStore(database)
	transfer := func(job workspacedomain.ProjectCopyJob, worker uuid.UUID, cleanup bool) *mediaapp.ProjectCopyTransfer {
		return mediaapp.NewProjectCopyTransfer(pgworkspace.NewProjectCopyMediaObjects(store, job.ID, worker, cleanup), mediaobjects.NewProjectCopyObjects(storage), "")
	}
	return workspaceapp.NewProjectCopyWorkerWithScript(store, transfer, provideProjectCopyScriptTransfer(database, storage), time.Now), store
}
