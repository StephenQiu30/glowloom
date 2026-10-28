package canvas

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

type assetAccess struct{ tx *gorm.DB }

// AssetAuthority binds asset reads to the current session and selected scope.
func AssetAuthority(tx *gorm.DB) canvasapp.AssetAccess { return assetAccess{tx: tx} }

func (a assetAccess) AuthorizeScope(ctx context.Context, actor identity.Principal) error {
	scope, err := scopewire.ProvideRequestScopeAccess(a.tx).Resolve(ctx, actor, actor.ScopeID, false)
	if err != nil {
		return socketScopeError(err)
	}
	if scope.PhysicalScopeID != actor.OrgID {
		return canvasapp.ErrNotFound
	}
	return nil
}

func (a assetAccess) AuthorizeProject(ctx context.Context, actor identity.Principal, id uuid.UUID) error {
	_, err := scopewire.ProvideProjectContentAccess(a.tx).Authorize(ctx, actor, id, false)
	return socketScopeError(err)
}

func (a assetAccess) AssetCoverAvailable(ctx context.Context, actor identity.Principal, project, id uuid.UUID) (bool, error) {
	fact, err := mediaapp.NewAssetQuery(pgmedia.NewScopedStore(a.tx, mediawire.NewMediaScopeAccess), nil).Reference(ctx, actor, project, id)
	if errors.Is(err, mediaapp.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return fact.ID == id && fact.ProjectID == project && fact.Kind == "image", nil
}
