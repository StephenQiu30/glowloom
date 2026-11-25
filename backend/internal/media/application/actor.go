package application

import (
	"errors"

	"github.com/google/uuid"
)

// Actor is Media's authenticated caller reference, without platform roles or grants.
// Identity/session and Creation ownership authorities remain separate live ports.
type Actor struct {
	ID              uuid.UUID
	OrgID           uuid.UUID
	ScopeID         uuid.UUID
	SessionID       uuid.UUID
	CredentialEpoch int64
	// PlatformEpoch is the original authenticated platform-qualification fact,
	// consumed only by the independent provider-logo authorization port.
	PlatformEpoch int64
}

var (
	// ErrSessionInvalid means the current authenticated caller reference is unavailable.
	ErrSessionInvalid = errors.New("media session unavailable")
	// ErrForbidden means the current owner authorities reject this operation.
	ErrForbidden = errors.New("media authority denied")
)
