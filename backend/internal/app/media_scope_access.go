package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
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
func NewCanvasProjectAccess(tx *gorm.DB) canvasapp.ProjectAccess { return mediaScopeAccess{tx: tx} }

func (a mediaScopeAccess) CheckActor(ctx context.Context, actor identity.Principal) error {
	if a.worker {
		return pgidentity.NewStore(a.tx).CheckActive(ctx, actor.ID)
	}
	return pgidentity.NewStore(a.tx).CheckSession(ctx, actor, time.Now)
}
func (a mediaScopeAccess) AuthorizePersonal(ctx context.Context, actor identity.Principal, write bool) error {
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
		s, err := provideScopeAccess(a.tx).Resolve(ctx, actor.ID, id, write)
		if err != nil {
			return mediaLibraryProjectError(err)
		}
		owner, kind, physical = s.OwnerUserID, s.Kind, s.PhysicalScopeID
	} else {
		s, err := provideRequestScopeAccess(a.tx).Resolve(ctx, actor, id, write)
		if err != nil {
			return mediaLibraryProjectError(err)
		}
		owner, kind, physical = s.OwnerUserID, s.Kind, s.PhysicalScopeID
	}
	if owner == nil || *owner != actor.ID || kind != "personal" || physical != actor.OrgID {
		return errors.Join(mediaapp.ErrNotFound, workspaceapp.ErrScopeNotFound)
	}
	return nil
}
func (a mediaScopeAccess) project() *pgworkspace.ProjectContentAccessStore {
	if a.worker {
		return provideWorkerProjectContentAccess(a.tx)
	}
	return provideProjectContentAccess(a.tx)
}
func (a mediaScopeAccess) AuthorizeProject(ctx context.Context, actor identity.Principal, id uuid.UUID, write bool) error {
	_, err := a.project().Authorize(ctx, actor, id, write)
	return mediaLibraryProjectError(err)
}
func (a mediaScopeAccess) AuthorizeCopyTarget(ctx context.Context, actor identity.Principal, id uuid.UUID) error {
	_, err := a.project().AuthorizeCopyTarget(ctx, actor, id)
	return mediaLibraryProjectError(err)
}

// TransferActor separates the private personal mapping from the project's scope.
func (a mediaScopeAccess) TransferActor(ctx context.Context, actor identity.Principal, library mediadomain.LibraryScope, write bool) (identity.Principal, error) {
	if err := a.CheckActor(ctx, actor); err != nil {
		return actor, err
	}
	if library.Kind == mediadomain.LibraryProject {
		facts, err := a.project().Authorize(ctx, actor, *library.ProjectID, write)
		if err != nil {
			return actor, mediaLibraryProjectError(err)
		}
		actor.OrgID = facts.OrgID
		if a.worker {
			id, err := a.project().OwnerScopeID(ctx, actor, *library.ProjectID, write)
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
		scope, err := provideScopeAccess(a.tx).Resolve(ctx, actor.ID, id, write)
		if err != nil {
			return actor, mediaLibraryProjectError(err)
		}
		actor.ScopeID, actor.OrgID = scope.ID, scope.PhysicalScopeID
	} else {
		scope, err := provideRequestScopeAccess(a.tx).Resolve(ctx, actor, id, write)
		if err != nil {
			return actor, mediaLibraryProjectError(err)
		}
		actor.ScopeID, actor.OrgID = scope.ID, scope.PhysicalScopeID
	}
	return actor, nil
}
