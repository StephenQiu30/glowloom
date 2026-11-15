package application

import (
	"errors"

	"github.com/google/uuid"
)

// Actor is Bible's authenticated account/session and Creation resource reference.
// It grants no role; each store rechecks current policy in its content transaction.
type Actor struct {
	ID, OrgID, ScopeID, SessionID uuid.UUID
	CredentialEpoch               int64
}

// ProjectFacts contains only the current project facts Bible actually consumes.
// The access implementation retains the owning project lock until the write ends.
type ProjectFacts struct {
	OwnerScopeID, ProjectID, OrgID uuid.UUID
	Revision                       int64
}

var (
	// ErrSessionInvalid rejects an absent, revoked or stale exact session.
	ErrSessionInvalid = errors.New("bible session invalid")
	// ErrForbidden rejects current Bible access without exposing foreign policy.
	ErrForbidden = errors.New("bible access forbidden")
)
