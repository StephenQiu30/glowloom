package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Access and coordination errors distinguish authentication, contention and dependency failures.
var (
	ErrSessionCSRFInvalid = fmt.Errorf("%w: invalid csrf proof", ErrSessionInvalid)
	ErrAccessInvalid      = errors.New("invalid access token")
	ErrAccessExpired      = errors.New("expired access token")
	ErrRefreshDependency  = errors.New("refresh coordination dependency unavailable")
	ErrRefreshBusy        = errors.New("refresh already in progress")
	ErrRefreshRateLimited = errors.New("refresh budget exceeded")
)

// AccessClaims contains identity references only; persistent authority remains in PostgreSQL.
type AccessClaims struct {
	UserID, SessionID, TokenID     uuid.UUID
	CredentialEpoch                int64
	IssuedAt, NotBefore, ExpiresAt time.Time
}

// AccessTokenCodec issues and authenticates short-lived access credentials.
type AccessTokenCodec interface {
	// CanSign reports configured signing capability without probing credentials.
	CanSign() bool
	Sign(AccessClaims) (string, error)
	Verify(string, time.Time) (AccessClaims, error)
}

// RefreshReceiptProtector binds recovery ciphertext to its exact refresh operation.
type RefreshReceiptProtector interface {
	Seal(uuid.UUID, int64, uuid.UUID, []byte) ([]byte, error)
	Open(uuid.UUID, int64, uuid.UUID, []byte) ([]byte, error)
}

// RefreshCoordinator coordinates attempts outside PostgreSQL transactions.
type RefreshCoordinator interface {
	Admit(context.Context, uuid.UUID, string) (func(context.Context) error, error)
	ReadReceipt(context.Context, uuid.UUID, int64) ([]byte, error)
	WriteReceipt(context.Context, uuid.UUID, int64, []byte, time.Time) error
	MarkRevoked(context.Context, uuid.UUID, time.Time) error
	IsRevoked(context.Context, uuid.UUID) (bool, error)
}
