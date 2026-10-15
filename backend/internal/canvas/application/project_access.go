package application

import (
	"context"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ProjectAccess authorizes exact project ownership without interpreting author IDs.
type ProjectAccess interface {
	AuthorizeProject(context.Context, identity.Principal, uuid.UUID, bool) error
	AuthorizeCopyTarget(context.Context, identity.Principal, uuid.UUID) error
}
