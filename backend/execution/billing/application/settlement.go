package application

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidSettlement means a terminal operation has incomplete or unsafe cost facts.
var ErrInvalidSettlement = errors.New("invalid operation settlement")

// SettleInput contains frozen facts supplied by the operation transaction
// coordinator. The caller owns the transaction and the terminal state write.
type SettleInput struct {
	ProjectID        uuid.UUID
	OperationID      uuid.UUID
	ActualCostMicros int64
	CapAtReservation bool
	Capability       string
	EpisodeID        *uuid.UUID
	ShotID           *uuid.UUID
	ModelKey         *string
	Region           *string
	OccurredAt       time.Time
}

// Validate requires an explicit scope and all dimensions that are supplied.
func (i SettleInput) Validate() error {
	if i.ProjectID == uuid.Nil || i.OperationID == uuid.Nil || i.ActualCostMicros < 0 ||
		strings.TrimSpace(i.Capability) == "" || len(i.Capability) > 256 || i.OccurredAt.IsZero() ||
		(i.EpisodeID != nil && *i.EpisodeID == uuid.Nil) ||
		(i.ShotID != nil && *i.ShotID == uuid.Nil) ||
		(i.ModelKey != nil && (strings.TrimSpace(*i.ModelKey) == "" || len(*i.ModelKey) > 256)) ||
		(i.Region != nil && *i.Region != "domestic" && *i.Region != "overseas") {
		return ErrInvalidSettlement
	}
	return nil
}

// SettleResult is the durable customer charge and platform paid excess.
type SettleResult struct {
	ChargeMicros          int64
	ReleasedMicros        int64
	ProviderOverageMicros int64
	BudgetOverrun         bool
	AlreadySettled        bool
}
