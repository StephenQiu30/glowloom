package application

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidReserveInput means the confirmation supplied incomplete billing facts.
var ErrInvalidReserveInput = errors.New("invalid operation reserve input")

// ReserveInput contains the quote amount and attribution frozen by the
// operation confirmation coordinator. The caller owns the transaction and
// must lock and recheck the quoted operation before reserving its amount.
type ReserveInput struct {
	ProjectID    uuid.UUID
	OperationID  uuid.UUID
	ActorID      uuid.UUID
	AmountMicros int64
	EpisodeID    *uuid.UUID
	ShotID       *uuid.UUID
	ModelKey     *string
	Region       *string
	OccurredAt   time.Time
}

// Validate rejects missing identities, negative amounts, and invalid dimensions.
func (i ReserveInput) Validate() error {
	if i.ProjectID == uuid.Nil || i.OperationID == uuid.Nil || i.ActorID == uuid.Nil ||
		i.AmountMicros < 0 || i.OccurredAt.IsZero() ||
		(i.EpisodeID != nil && *i.EpisodeID == uuid.Nil) ||
		(i.ShotID != nil && *i.ShotID == uuid.Nil) ||
		(i.ModelKey != nil && (strings.TrimSpace(*i.ModelKey) == "" || len(*i.ModelKey) > 256)) ||
		(i.Region != nil && *i.Region != "domestic" && *i.Region != "overseas") {
		return ErrInvalidReserveInput
	}
	return nil
}

// ReserveResult identifies the held amount and balance after the transaction.
type ReserveResult struct {
	ReservationID   uuid.UUID
	AvailableMicros int64
	BudgetRevision  int64
}
