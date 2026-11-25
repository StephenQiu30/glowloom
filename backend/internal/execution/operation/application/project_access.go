package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ProjectAccess checks the current request or durable worker actor in the caller transaction.
type ProjectAccess interface {
	AuthorizeOperation(context.Context, identity.Principal, uuid.UUID, bool) error
}

// PlatformAccess checks the current exact platform session for governance commands.
type PlatformAccess interface {
	CheckPlatformSession(context.Context, identity.Principal, func() time.Time) error
}

// ErrExecutionAuthorityRevoked stops execution without retrying or dropping supplier facts.
var ErrExecutionAuthorityRevoked = errors.New("operation execution authority revoked")
