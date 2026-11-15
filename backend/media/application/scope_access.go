package application

import (
	"context"

	"github.com/StephenQiu30/glowloom-server/backend/media/domain"

	"github.com/google/uuid"
)

// ScopeAccess retains live identity and exact ownership locks in the caller transaction.
// Request and worker implementations are composed separately; roles are never authority.
type ScopeAccess interface {
	CheckActor(context.Context, Actor) error
	AuthorizePersonal(context.Context, Actor, bool) error
	AuthorizeProject(context.Context, Actor, uuid.UUID, bool) error
	AuthorizeCopyTarget(context.Context, Actor, uuid.UUID) error
}

// TransferScopeAccess resolves each side independently. Personal resources always
// belong to the current account; project resources retain the exact selected scope.
type TransferScopeAccess interface {
	TransferActor(context.Context, Actor, domain.LibraryScope, bool) (Actor, error)
}
