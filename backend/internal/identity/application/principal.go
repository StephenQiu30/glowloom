// Package application coordinates authentication and organization-scoped account commands.
package application

import (
 "encoding/json"
 "errors"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// Principal identifies the actor recorded by organization-scoped commands.
// The public API resolves it from a valid server-side session.
type Principal struct {
	ID                 uuid.UUID
	SessionID          uuid.UUID
	CredentialEpoch    int64
	PlatformEpoch      int64
	PlatformRole       string
	ScopeID            uuid.UUID
	OrgID              uuid.UUID
	LoginName          string
	DisplayName        string
	Role               domain.Role
	MustChangePassword bool
}

// ErrForbidden means current authority does not permit the operation.
var ErrForbidden = errors.New("forbidden")

// OutboxEvent is the retained event envelope used by content and audit consumers.
type OutboxEvent struct {
 ID uuid.UUID
 Topic string
 PartitionKey string
 Payload json.RawMessage
}
