package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrSessionNotFound covers absent, revoked, and expired session tokens.
var ErrSessionNotFound = errors.New("session not found")

// Session is the identity and deadline state stored for an authenticated user.
// Callers must compare SessionEpoch with the current user before accepting it.
type Session struct {
	UserID       uuid.UUID `json:"user_id"`
	OrgID        uuid.UUID `json:"org_id"`
	SessionEpoch int64     `json:"session_epoch"`
	CreateTime   time.Time `json:"create_time"`
	LastSeen     time.Time `json:"last_seen"`
}
