package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ActorAccess checks exact platform sessions and current asynchronous platform authority.
type ActorAccess interface {
	CheckPlatformSession(context.Context, identity.Principal, func() time.Time) error
	CheckActive(context.Context, uuid.UUID) error
}

// ProjectAccess checks model availability in the current exact creation scope.
type ProjectAccess interface {
	AuthorizeCatalog(context.Context, identity.Principal, uuid.UUID) error
}
