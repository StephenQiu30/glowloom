// Package operation wires operation authority and persistence dependencies.
package operation

import (
	"context"
	"errors"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"

	promptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/prompt"

	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"

	"github.com/google/uuid"
	"gorm.io/gorm"

	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"

	pgprompt "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/postgres"

	workspace "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application"

	pgcatalog "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/adapter/postgres"

	catalog "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/application"

	pgoperation "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/postgres"

	operation "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

type operationProjectAccess struct {
	worker bool
	scopes workspace.RequestScopeResolver
	store  interface {
		Authorize(context.Context, identity.Principal, uuid.UUID, bool) (workspace.ProjectContentAccess, error)
	}
}

func (a operationProjectAccess) AuthorizeOperation(ctx context.Context, actor identity.Principal, project uuid.UUID, write bool) error {
	_, err := a.store.Authorize(ctx, actor, project, write)
	if a.worker && (errors.Is(err, identity.ErrForbidden) || errors.Is(err, workspacesource.ErrScopeNotFound) || errors.Is(err, workspacesource.ErrProjectNotFound)) {
		return operation.ErrExecutionAuthorityRevoked
	}
	if errors.Is(err, workspacesource.ErrScopeNotFound) || errors.Is(err, workspacesource.ErrProjectNotFound) {
		return errors.Join(operation.ErrPublicNotFound, err)
	}
	return err
}
func (a operationProjectAccess) AuthorizeBilling(ctx context.Context, actor identity.Principal, project uuid.UUID, write bool) error {
	return a.AuthorizeOperation(ctx, actor, project, write)
}

// CanPublishHistory checks the current exact session and writable scope under
// share locks. It does not turn a list query into a project content mutation.
func (a operationProjectAccess) CanPublishHistory(ctx context.Context, actor identity.Principal) (bool, error) {
	if a.worker || a.scopes == nil {
		return false, nil
	}
	scope, err := a.scopes.Resolve(ctx, actor, actor.ScopeID, true)
	if errors.Is(err, workspacesource.ErrScopeNotFound) || errors.Is(err, identity.ErrForbidden) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return scope.ID == actor.ScopeID && scope.PhysicalScopeID == actor.OrgID, nil
}

// ContentProjectAccess adapts a project-content store for callers outside this package.
func ContentProjectAccess(store interface {
	Authorize(context.Context, identity.Principal, uuid.UUID, bool) (workspace.ProjectContentAccess, error)
}) operation.ProjectAccess {
	return operationProjectAccess{store: store}
}

// ProvideOperationProjectAccess adapts request project authority for operations.
func ProvideOperationProjectAccess(tx *gorm.DB) operation.ProjectAccess {
	return operationProjectAccess{store: scopewire.ProvideProjectContentAccess(tx), scopes: scopewire.ProvideRequestScopeAccess(tx)}
}

// ProvideOperationWorkerAccess adapts worker project authority for operations.
func ProvideOperationWorkerAccess(tx *gorm.DB) operation.ProjectAccess {
	return operationProjectAccess{store: scopewire.ProvideWorkerProjectContentAccess(tx), worker: true}
}

// ProvideOperationStore assembles operation persistence and cross-module dependencies.
func ProvideOperationStore(db *gorm.DB) *pgoperation.Store {
	return pgoperation.NewScopedStore(db, ProvideOperationProjectAccess, ProvideOperationWorkerAccess, ProvideOperationPlatformAccess).WithExecutionAvailability(func(tx *gorm.DB) operation.ExecutionAvailability { return pgcatalog.NewStore(tx) }).WithSubjectMedia(func(tx *gorm.DB) operation.SubjectMediaAccess {
		return subjectwire.NewGenerationAccess(tx, subjectwire.Objects{})
	})
}

// ProvidePromptPreferenceStore assembles prompt preferences with operation authority.
func ProvidePromptPreferenceStore(db *gorm.DB) *pgprompt.Store {
	return pgprompt.NewScopedStore(db, promptwire.SessionAccess)
}

func (a operationProjectAccess) AuthorizeCatalog(ctx context.Context, actor identity.Principal, project uuid.UUID) error {
	err := a.AuthorizeOperation(ctx, actor, project, false)
	if errors.Is(err, operation.ErrPublicNotFound) {
		return errors.Join(catalog.ErrModelCatalogProjectNotFound, err)
	}
	return err
}

// NewCatalogStore composes current platform sessions and project scope authorization.
func NewCatalogStore(db *gorm.DB) *pgcatalog.Store {
	return pgcatalog.NewScopedStore(db, func(tx *gorm.DB) catalog.ActorAccess { return pgidentity.NewStore(tx) }, func(tx *gorm.DB) catalog.ProjectAccess {
		return operationProjectAccess{store: scopewire.ProvideProjectContentAccess(tx), scopes: scopewire.ProvideRequestScopeAccess(tx)}
	}).WithCheckUsage(func(tx *gorm.DB) catalog.ProviderCheckUsageReader { return pgoperation.NewStore(tx) })
}

// ProvideOperationPlatformAccess adapts platform authority for operations.
func ProvideOperationPlatformAccess(tx *gorm.DB) operation.PlatformAccess {
	return pgidentity.NewStore(tx)
}
