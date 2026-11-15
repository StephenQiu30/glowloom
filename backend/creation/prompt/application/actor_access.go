package application

import (
	"context"
	"time"
)

// ActorAccess retains the exact account session in the preference transaction.
type ActorAccess interface {
	CheckSession(context.Context, Actor, func() time.Time) error
}
