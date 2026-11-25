package media

import (
	"context"
	"errors"
	"time"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/application/source"

	"github.com/google/uuid"
	"gorm.io/gorm"

	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"

	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/creation/canvas/application"

	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/adapter/postgres"

	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/application"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"

	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type mediaScopeAccess struct {
	tx     *gorm.DB
	worker bool
}

// NewMediaScopeAccess binds a media request to the current session and creation scope.
func NewMediaScopeAccess(tx *gorm.DB) mediaapp.ScopeAccess { return mediaScopeAccess{tx: tx} }

// NewMediaWorkerScopeAccess resolves current durable authority for asynchronous media work.
func NewMediaWorkerScopeAccess(tx *gorm.DB) mediaapp.ScopeAccess {
	return mediaScopeAccess{tx: tx, worker: true}
}

// NewCanvasProjectAccess binds canvas access to the current request authority.
func NewCanvasProjectAccess(tx *gorm.DB) canvasapp.ProjectAccess {
	return canvasProjectAccess{mediaScopeAccess{tx: tx}}
}

func (a mediaScopeAccess) CheckActor(ctx context.Context, actor mediaapp.Actor) error {
	if a.worker {
		return OwningError(pgidentity.NewStore(a.tx).CheckActive(ctx, actor.ID))
	}
	return OwningError(pgidentity.NewStore(a.tx).CheckSession(ctx, IdentityActor(actor), time.Now))
}
func (a mediaScopeAccess) AuthorizePersonal(ctx context.Context, actor mediaapp.Actor, write bool) error {
	id := actor.ScopeID
	if a.worker {
		var err error
		id, err = pgworkspace.NewScopeStore(a.tx, func(tx *gorm.DB) workspaceapp.ActorAccess { return pgidentity.NewStore(tx) }).PersonalScopeID(ctx, actor.ID)
		if err != nil {
			return mediaLibraryProjectError(err)
		}
	}
	var owner *uuid.UUID
	var kind string
	var physical uuid.UUID
	if a.worker {
		s, err := scopewire.ProvideScopeAccess(a.tx).Resolve(ctx, actor.ID, id, write)
		if err != nil {
			return mediaLibraryProjectError(err)
		}
		owner, kind, physical = s.OwnerUserID, s.Kind, s.PhysicalScopeID
	} else {
		s, err := scopewire.ProvideRequestScopeAccess(a.tx).Resolve(ctx, IdentityActor(actor), id, write)
		if err != nil {
			return mediaLibraryProjectError(err)
		}
		owner, kind, physical = s.OwnerUserID, s.Kind, s.PhysicalScopeID
	}
	if owner == nil || *owner != actor.ID || kind != "personal" || physical != actor.OrgID {
		return errors.Join(mediaapp.ErrNotFound, workspacesource.ErrScopeNotFound)
	}
	return nil
}
func (a mediaScopeAccess) project() *pgworkspace.ProjectContentAccessStore {
	if a.worker {
		return scopewire.ProvideWorkerProjectContentAccess(a.tx)
	}
	return scopewire.ProvideProjectContentAccess(a.tx)
}
func (a mediaScopeAccess) AuthorizeProject(ctx context.Context, actor mediaapp.Actor, id uuid.UUID, write bool) error {
	_, err := a.project().Authorize(ctx, IdentityActor(actor), id, write)
	return mediaLibraryProjectError(err)
}
func (a mediaScopeAccess) AuthorizeCopyTarget(ctx context.Context, actor mediaapp.Actor, id uuid.UUID) error {
	_, err := a.project().AuthorizeCopyTarget(ctx, IdentityActor(actor), id)
	return mediaLibraryProjectError(err)
}

// TransferActor separates the private personal mapping from the project's scope.
func (a mediaScopeAccess) TransferActor(ctx context.Context, actor mediaapp.Actor, library mediadomain.LibraryScope, write bool) (mediaapp.Actor, error) {
	if err := a.CheckActor(ctx, actor); err != nil {
		return actor, err
	}
	if library.Kind == mediadomain.LibraryProject {
		facts, err := a.project().Authorize(ctx, IdentityActor(actor), *library.ProjectID, write)
		if err != nil {
			return actor, mediaLibraryProjectError(err)
		}
		actor.OrgID = facts.OrgID
		if a.worker {
			id, err := a.project().OwnerScopeID(ctx, IdentityActor(actor), *library.ProjectID, write)
			if err != nil {
				return actor, err
			}
			actor.ScopeID = id
		}
		return actor, nil
	}
	if library.Kind != mediadomain.LibraryPersonal {
		return actor, mediaapp.ErrNotFound
	}
	id, err := pgworkspace.NewScopeStore(a.tx, func(tx *gorm.DB) workspaceapp.ActorAccess { return pgidentity.NewStore(tx) }).PersonalScopeID(ctx, actor.ID)
	if err != nil {
		return actor, err
	}
	if a.worker {
		scope, err := scopewire.ProvideScopeAccess(a.tx).Resolve(ctx, actor.ID, id, write)
		if err != nil {
			return actor, mediaLibraryProjectError(err)
		}
		actor.ScopeID, actor.OrgID = scope.ID, scope.PhysicalScopeID
	} else {
		scope, err := scopewire.ProvideRequestScopeAccess(a.tx).Resolve(ctx, IdentityActor(actor), id, write)
		if err != nil {
			return actor, mediaLibraryProjectError(err)
		}
		actor.ScopeID, actor.OrgID = scope.ID, scope.PhysicalScopeID
	}
	return actor, nil
}
