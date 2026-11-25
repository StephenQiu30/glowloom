package application

import (
	"errors"

	"github.com/google/uuid"
)

// Actor identifies a caller; current access remains the owning port's decision.
// Roles and grants are never copied into Script as local authority.
type Actor struct {
	ID, OrgID, ScopeID, SessionID uuid.UUID
	CredentialEpoch               int64
}

// ProjectFacts are the project facts required under the caller's transaction.
type ProjectFacts struct {
	OwnerScopeID, ProjectID, OrgID uuid.UUID
	Revision                       int64
}

var (
	// ErrSessionInvalid means the caller's exact session cannot authorize work.
	ErrSessionInvalid = errors.New("script session invalid")
	// ErrForbidden means current owning policy denies this action.
	ErrForbidden = errors.New("script access forbidden")
)
