package application

import (
	"context"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ProjectAccess checks current request or worker authority in the billing transaction.
type ProjectAccess interface {
	AuthorizeBilling(context.Context, identity.Principal, uuid.UUID, bool) error
}
