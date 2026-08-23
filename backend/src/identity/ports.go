package identity

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
	Workspace   string
}

type PersistedRegisterInput struct {
	Email        EmailAddress
	PasswordHash string
	DisplayName  string
	Workspace    string
}

type LoginAccount struct {
	Identity     AuthIdentity
	PasswordHash string
}

type IdentityStore interface {
	RegisterAccount(context.Context, PersistedRegisterInput) (SessionIssue, error)
	FindLoginAccount(context.Context, EmailAddress) (LoginAccount, error)
	CreateSession(context.Context, AuthIdentity) (SessionIssue, error)
	RotateRefreshSession(context.Context, string) (SessionIssue, error)
	RevokeRefreshSession(context.Context, string) (uuid.UUID, error)
	Authenticate(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (Principal, error)
	AuthorizePath(context.Context, uuid.UUID, string) error
	ListWorkspaceMembers(context.Context, uuid.UUID, WorkspaceMemberQuery) (WorkspaceMemberPage, error)
	UpdateWorkspaceMember(context.Context, uuid.UUID, uuid.UUID, Principal, WorkspaceMemberUpdate) (WorkspaceMember, error)
}

// IdentityCache is implemented by the platform Redis adapter. The identity
// module owns the port so business code does not depend on Redis result types.
type IdentityCache interface {
	AllowIdentityGCRA(context.Context, string, int64, time.Duration, int64) (bool, time.Duration, int64, error)
	IdentityGet(context.Context, string) (string, bool, error)
	IdentitySet(context.Context, string, string, time.Duration) error
	IdentitySetNX(context.Context, string, string, time.Duration) (bool, error)
	IdentityCompareAndDelete(context.Context, string, string) error
}
