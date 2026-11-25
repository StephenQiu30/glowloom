package application

import (
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

// Actor is Prompt's minimum authenticated account/session and resource reference.
// It contains no platform role or grant. Stores must recheck current policy in
// their own content transaction; constructing this value never grants a permit.
type Actor struct {
	ID, OrgID, ScopeID, SessionID uuid.UUID
	CredentialEpoch               int64
}

var (
	// ErrSessionInvalid rejects an absent, revoked or stale exact session.
	ErrSessionInvalid = errors.New("prompt session invalid")
	// ErrForbidden rejects current Prompt access without leaking unrelated policy.
	ErrForbidden = errors.New("prompt access forbidden")
)

// PreferenceAudit is the one durable audit fact emitted by personal preference
// changes. Its topic and organization partition are fixed by Prompt persistence.
type PreferenceAudit struct {
	ID      uuid.UUID
	Payload json.RawMessage
}
