package operation

import (
	"context"
	"errors"

	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgcatalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/postgres"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	pgprompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/adapter/postgres"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

type operationProjectAccess struct {
	worker bool
	store  interface {
		Authorize(context.Context, identity.Principal, uuid.UUID, bool) (workspace.ProjectContentAccess, error)
	}
}

func (a operationProjectAccess) AuthorizeOperation(ctx context.Context, actor identity.Principal, project uuid.UUID, write bool) error {
	_, err := a.store.Authorize(ctx, actor, project, write)
	if a.worker && (errors.Is(err, identity.ErrForbidden) || errors.Is(err, workspace.ErrScopeNotFound) || errors.Is(err, workspace.ErrProjectNotFound)) {
		return operation.ErrExecutionAuthorityRevoked
	}
	return err
}
func (a operationProjectAccess) AuthorizeBilling(ctx context.Context, actor identity.Principal, project uuid.UUID, write bool) error {
	return a.AuthorizeOperation(ctx, actor, project, write)
}

// ContentProjectAccess adapts a project-content store for callers outside this package.
func ContentProjectAccess(store interface {
	Authorize(context.Context, identity.Principal, uuid.UUID, bool) (workspace.ProjectContentAccess, error)
}) operation.ProjectAccess {
	return operationProjectAccess{store: store}
}

func ProvideOperationProjectAccess(tx *gorm.DB) operation.ProjectAccess {
	return operationProjectAccess{store: scopewire.ProvideProjectContentAccess(tx)}
}
func ProvideOperationWorkerAccess(tx *gorm.DB) operation.ProjectAccess {
	return operationProjectAccess{store: scopewire.ProvideWorkerProjectContentAccess(tx), worker: true}
}

func ProvideOperationStore(db *gorm.DB) *pgoperation.Store {
	return pgoperation.NewScopedStore(db, ProvideOperationProjectAccess, ProvideOperationWorkerAccess, ProvideOperationPlatformAccess)
}

func ProvidePromptPreferenceStore(db *gorm.DB) *pgprompt.Store {
	return pgprompt.NewScopedStore(db, func(tx *gorm.DB) prompt.ActorAccess { return pgidentity.NewStore(tx) })
}

func (a operationProjectAccess) AuthorizeCatalog(ctx context.Context, actor identity.Principal, project uuid.UUID) error {
	return a.AuthorizeOperation(ctx, actor, project, false)
}

// NewCatalogStore composes current platform sessions and project scope authorization.
func NewCatalogStore(db *gorm.DB) *pgcatalog.Store {
	return pgcatalog.NewScopedStore(db, func(tx *gorm.DB) catalog.ActorAccess { return pgidentity.NewStore(tx) }, func(tx *gorm.DB) catalog.ProjectAccess {
		return operationProjectAccess{store: scopewire.ProvideProjectContentAccess(tx)}
	})
}

func ProvideOperationPlatformAccess(tx *gorm.DB) operation.PlatformAccess {
	return pgidentity.NewStore(tx)
}
