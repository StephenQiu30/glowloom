package application

import (
	"context"
	"time"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ActorAccess retains the exact account session in the preference transaction.
type ActorAccess interface {
	CheckSession(context.Context, identity.Principal, time.Time) error
}
