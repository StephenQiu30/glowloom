package domain

import (
	"time"

	"github.com/google/uuid"
)

// RefreshToken retains only a digest and a short lived opaque encrypted receipt.
// AccountID is projected from the parent session, never independently persisted.
type RefreshToken struct {
	ID                uuid.UUID
	SessionID         uuid.UUID
	AccountID         uuid.UUID
	RotationRequestID uuid.UUID
	Generation        int64
	Digest            []byte
	ReceiptCiphertext []byte
	IssuedAt          time.Time
	ConsumedAt        *time.Time
	RevokedAt         *time.Time
	ReplayUntil       *time.Time
}
