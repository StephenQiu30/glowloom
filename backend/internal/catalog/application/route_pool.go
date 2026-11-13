package application

import (
	"context"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/google/uuid"
)

// RouteOption identifies a published model release on a particular connection.
type RouteOption struct {
	ReleaseID       uuid.UUID `json:"release_id"`
	ProviderName    string    `json:"provider_name"`
	ProviderModelID string    `json:"provider_model_id"`
	DisplayName     string    `json:"display_name"`
}

// RoutePool is the ordered, reviewed list of backup releases and eligible options.
type RoutePool struct {
	Recent   []RouteAttempt `json:"recent,omitempty"`
	Revision int            `json:"revision"`
	Backups  []uuid.UUID    `json:"backups"`
	Options  []RouteOption  `json:"options,omitempty"`
}

// PoolPublication describes one compare-and-swap publication.
type PoolPublication struct {
	ExpectedRevision int         `json:"expected_revision"`
	Backups          []uuid.UUID `json:"backups"`
	Equivalent       bool        `json:"equivalent"`
	RequestID        uuid.UUID   `json:"request_id"`
}

// RoutePoolStore authorizes the current administrator and owns atomic publication.
type RoutePoolStore interface {
	SetPoolKey(context.Context, identity.Principal, uuid.UUID, uuid.UUID, bool, uuid.UUID) (ProviderCredentialSummary, error)
	ReadRoutePool(context.Context, identity.Principal, uuid.UUID) (RoutePool, error)
	PublishRoutePool(context.Context, identity.Principal, uuid.UUID, PoolPublication) (RoutePool, error)
}

// RouteAttempt identifies the actual route without disclosing credentials.
type RouteAttempt struct {
	OperationID  uuid.UUID `json:"operation_id"`
	Attempt      int       `json:"attempt"`
	ProviderName string    `json:"provider_name"`
	KeySuffix    string    `json:"key_suffix"`
	Reason       string    `json:"reason"`
}
