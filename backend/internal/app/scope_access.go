package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

type scopeActorAccess struct{ store *pgidentity.Store }

func (a scopeActorAccess) CheckActive(ctx context.Context, id uuid.UUID) error {
	err := a.store.CheckActive(ctx, id)
	if errors.Is(err, identityapp.ErrForbidden) {
		return workspaceapp.ErrScopeNotFound
	}
	return err
}

func provideScopeAccess(database *gorm.DB) *workspaceapp.ScopeAccess {
	return workspaceapp.NewScopeAccess(pgworkspace.NewScopeStore(database, func(tx *gorm.DB) workspaceapp.ActorAccess { return scopeActorAccess{store: pgidentity.NewStore(tx)} }))
}

// provideRequestScopeAccess keeps identity and membership locks in the consumer transaction.
func provideRequestScopeAccess(tx *gorm.DB) workspaceapp.RequestScopeResolver {
	return workspaceapp.NewRequestScopeAccess(pgworkspace.NewRequestScopeStore(tx, func(ownerTx *gorm.DB) workspaceapp.SessionActorAccess {
		return pgidentity.NewStore(ownerTx)
	}), time.Now)
}

func provideProjectContentAccess(tx *gorm.DB) *pgworkspace.ProjectContentAccessStore {
	return pgworkspace.NewScopedProjectContentAccessStore(tx, provideRequestScopeAccess(tx))
}

func provideWorkerProjectContentAccess(tx *gorm.DB) *pgworkspace.ProjectContentAccessStore {
	return pgworkspace.NewWorkerProjectContentAccessStore(tx, provideScopeAccess(tx))
}
