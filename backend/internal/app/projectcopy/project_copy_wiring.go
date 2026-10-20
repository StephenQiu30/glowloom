package projectcopy

import (
	"context"
	"time"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	workspacewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/workspace"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgbilling "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	workspacedomain "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
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

// ProvideProjectCopyStore assembles request-authorized project copy storage.
func ProvideProjectCopyStore(database *gorm.DB) *pgworkspace.ProjectCopyStore {
	return ProvideProjectCopyStoreWithAccess(database, mediawire.NewMediaScopeAccess)
}

// ProvideProjectCopyStoreWithAccess assembles project copy storage with explicit scope access.
func ProvideProjectCopyStoreWithAccess(database *gorm.DB, scopes pgmedia.ScopeAccessFactory) *pgworkspace.ProjectCopyStore {
	return pgworkspace.NewScopedProjectCopyStoreWithBible(database, scopewire.ProvideRequestScopeAccess, provideCopyWorkerScopeAccess, workspacewire.ProvideProjectWorkGuard,
		func(tx *gorm.DB) workspaceapp.ProjectCopyOwners {
			media := pgmedia.NewScopedProjectCopyStore(tx, scopes)
			canvas := pgcanvas.NewScopedProjectCopyStore(tx,
				func(mediaTx *gorm.DB) canvasapp.MediaReader {
					return mediaapp.NewAssetQuery(pgmedia.NewScopedStore(mediaTx, scopes), nil)
				},
				func(mediaTx *gorm.DB, binding canvasapp.ProjectCopyBinding) canvasapp.MediaReader {
					return copiedCanvasMedia{store: pgmedia.NewScopedProjectCopyStore(mediaTx, scopes), binding: mediaapp.ProjectCopyBinding{
						JobID: binding.JobID, OrgID: binding.OrgID, SourceProjectID: binding.SourceProjectID, TargetProjectID: binding.TargetProjectID,
					}}
				}, func(tx *gorm.DB) canvasapp.ProjectAccess {
					return mediawire.NewCanvasProjectAccessWithScope(tx, scopes(tx))
				})
			return workspaceapp.ProjectCopyOwners{Media: media, Canvas: canvas, Budget: pgbilling.NewScopedStore(tx, func(db *gorm.DB) billingapp.ProjectAccess { return copyBudgetAccess{scopes(db)} }), Cover: projectCopyCoverOwner{tx: tx, scopes: scopes}}
		}, func(tx *gorm.DB, authority workspaceapp.ProjectCopyAuthority) workspaceapp.ProjectCopyScriptOwner {
			return projectCopyScriptOwner{store: provideScriptProjectCopyStore(tx, authority)}
		}, func(tx *gorm.DB, authority workspaceapp.ProjectCopyAuthority, mediaSnapshot mediaapp.ProjectCopySnapshot) workspaceapp.ProjectCopyBibleOwner {
			return projectCopyBibleOwner{store: provideBibleProjectCopyStore(tx, authority, mediaSnapshot)}
		})
}

// ProvideProjectCopyWorker assembles the project copy worker and its store.
func ProvideProjectCopyWorker(database *gorm.DB, storage *objectstorage.Client) (*workspaceapp.ProjectCopyWorker, *pgworkspace.ProjectCopyStore) {
	store := ProvideProjectCopyStoreWithAccess(database, mediawire.NewMediaWorkerScopeAccess)
	transfer := func(job workspacedomain.ProjectCopyJob, worker uuid.UUID, cleanup bool) *mediaapp.ProjectCopyTransfer {
		return mediaapp.NewProjectCopyTransfer(pgworkspace.NewProjectCopyMediaObjects(store, job.ID, worker, cleanup), mediaobjects.NewProjectCopyObjects(storage), "")
	}
	return workspaceapp.NewProjectCopyWorkerWithBible(store, transfer, provideProjectCopyScriptTransfer(database, storage), provideProjectCopyBibleTransfer(database, storage), time.Now), store
}

type copyBudgetAccess struct{ access mediaapp.ScopeAccess }

func (a copyBudgetAccess) AuthorizeBilling(ctx context.Context, actor identityapp.Principal, project uuid.UUID, _ bool) error {
	return a.access.AuthorizeCopyTarget(ctx, actor, project)
}
