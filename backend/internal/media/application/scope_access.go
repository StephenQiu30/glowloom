package application

import (
	"context"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ScopeAccess retains live identity and exact ownership locks in the caller transaction.
// Request and worker implementations are composed separately; roles are never authority.
type ScopeAccess interface {
	CheckActor(context.Context, identity.Principal) error
	AuthorizePersonal(context.Context, identity.Principal, bool) error
	AuthorizeProject(context.Context, identity.Principal, uuid.UUID, bool) error
	AuthorizeCopyTarget(context.Context, identity.Principal, uuid.UUID) error
}

// TransferScopeAccess resolves each side independently. Personal resources always
// belong to the current account; project resources retain the exact selected scope.
type TransferScopeAccess interface {
	TransferActor(context.Context, identity.Principal, domain.LibraryScope, bool) (identity.Principal, error)
}
