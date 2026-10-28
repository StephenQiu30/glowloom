package canvas

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	workspacewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/workspace"
	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	workspacedomain "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

type assetMutationAccess struct {
	tx      *gorm.DB
	runtime canvasapp.YjsRuntime
}

// AssetMutationAuthority preserves the existing module owners under the command transaction.
func AssetMutationAuthority(tx *gorm.DB) canvasapp.AssetMutationAccess {
	return assetMutationAccess{tx: tx}
}

// NewAssetMutationAuthority supplies the private runtime only for initialized titles.
func NewAssetMutationAuthority(runtime canvasapp.YjsRuntime) pgcanvas.AssetMutationFactory {
	return func(tx *gorm.DB) canvasapp.AssetMutationAccess { return assetMutationAccess{tx: tx, runtime: runtime} }
}

func (a assetMutationAccess) AuthorizeAssetCommand(ctx context.Context, actor identity.Principal, write bool) error {
	scope, err := scopewire.ProvideRequestScopeAccess(a.tx).Resolve(ctx, actor, actor.ScopeID, write)
	if err != nil {
		return socketScopeError(err)
	}
	if scope.PhysicalScopeID != actor.OrgID {
		return canvasapp.ErrNotFound
	}
	return nil
}

func (a assetMutationAccess) AuthorizeAssetProject(ctx context.Context, actor identity.Principal, id uuid.UUID, write bool) error {
	_, err := scopewire.ProvideProjectContentAccess(a.tx).Authorize(ctx, actor, id, write)
	return assetMutationError(err)
}

func assetMutationError(err error) error {
	switch {
	case errors.Is(err, workspaceapp.ErrScopeNotFound), errors.Is(err, workspaceapp.ErrProjectNotFound), errors.Is(err, mediaapp.ErrNotFound):
		return canvasapp.ErrNotFound
	case errors.Is(err, workspacedomain.ErrProjectRevisionConflict):
		return &canvasapp.AssetActionError{Code: "project_revision_conflict"}
	case errors.Is(err, workspacedomain.ErrProjectStateConflict):
		return &canvasapp.AssetActionError{Code: "project_state_conflict"}
	case errors.Is(err, workspacedomain.ErrProjectHasInflightOperations):
		return &canvasapp.AssetActionError{Code: "project_has_inflight_operations"}
	default:
		return err
	}
}

func (a assetMutationAccess) RenameAsset(ctx context.Context, actor identity.Principal, facts canvasapp.AssetMutationFacts, key uuid.UUID, item canvasapp.AssetCommandItem, name string) (canvasapp.AssetCommandResult, error) {
	result := canvasapp.AssetCommandResult{ID: facts.ID, Revision: canvasapp.Revision(facts.Revision), ProjectRevision: facts.ProjectRevision}
	if facts.IsMain {
		if item.ExpectedProjectRevision < 1 {
			return result, &canvasapp.AssetActionError{Code: "project_revision_required"}
		}
		saved, err := workspaceapp.NewProjectLifecycle(workspacewire.ProvideWorkspaceLifecycleStore(a.tx), time.Now).Change(ctx, actor, workspaceapp.ProjectChangeInput{
			Action: "patch", IdempotencyKey: key,
			Patch: workspaceapp.UpdateProjectInput{ProjectID: facts.ProjectID, ExpectedRevision: item.ExpectedProjectRevision, Name: &name, RequestID: key.String()},
		})
		if err != nil {
			return result, assetMutationError(err)
		}
		result.ProjectRevision = saved.Project.Revision
		return result, nil
	}
	store := pgcanvas.NewScopedStore(a.tx, nil, mediawire.NewCanvasProjectAccess)
	state, err := store.LoadYjs(ctx, actor, facts.ID)
	if err != nil {
		return result, err
	}
	if len(state.State) > 0 {
		saved, err := canvasapp.NewYjsService(store, a.runtime).RenameAsset(ctx, actor, facts.ID, key, item.ExpectedRevision, name)
		if err != nil {
			return result, err
		}
		if saved.Status != "accepted" {
			return result, &canvasapp.AssetActionError{Code: saved.Code}
		}
		result.Revision = saved.ContentRevision
		return result, nil
	}
	doc, err := canvasapp.NewService(pgcanvas.NewScopedStore(a.tx, nil, mediawire.NewCanvasProjectAccess)).Rename(ctx, actor, facts.ID, key.String(), canvasapp.RenameInput{ExpectedRevision: int64(item.ExpectedRevision), Name: name})
	if err == nil {
		result.Revision = canvasapp.Revision(doc.Revision)
	}
	return result, assetMutationError(err)
}

func (a assetMutationAccess) DeleteAsset(ctx context.Context, actor identity.Principal, facts canvasapp.AssetMutationFacts, key uuid.UUID, item canvasapp.AssetCommandItem) (canvasapp.AssetCommandResult, error) {
	result := canvasapp.AssetCommandResult{ID: facts.ID, Revision: canvasapp.Revision(facts.Revision), ProjectRevision: facts.ProjectRevision}
	if facts.IsMain {
		if item.ExpectedProjectRevision < 1 {
			return result, &canvasapp.AssetActionError{Code: "project_revision_required"}
		}
		saved, err := workspaceapp.NewProjectLifecycle(workspacewire.ProvideWorkspaceLifecycleStore(a.tx), time.Now).Change(ctx, actor, workspaceapp.ProjectChangeInput{
			Action: "delete", IdempotencyKey: key,
			Patch: workspaceapp.UpdateProjectInput{ProjectID: facts.ProjectID, ExpectedRevision: item.ExpectedProjectRevision, RequestID: key.String()},
		})
		if err != nil {
			return result, assetMutationError(err)
		}
		result.ProjectRevision = saved.Project.Revision
		return result, nil
	}
	deleted, err := canvasapp.NewService(pgcanvas.NewScopedStore(a.tx, nil, mediawire.NewCanvasProjectAccess)).Delete(ctx, actor, facts.ID, key.String(), canvasapp.DeleteInput{ExpectedRevision: int64(item.ExpectedRevision)})
	if err == nil {
		result.Revision = canvasapp.Revision(deleted.Revision)
	}
	return result, assetMutationError(err)
}
