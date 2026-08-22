package identity

import (
	"context"

	"github.com/google/uuid"
)

type IdentityStore interface {
	CreateSession(context.Context, string, uuid.UUID) (Session, error)
	Authenticate(context.Context, string, uuid.UUID) (Principal, error)
	Revoke(context.Context, string) error
	AuthorizePath(context.Context, uuid.UUID, string) error
}
