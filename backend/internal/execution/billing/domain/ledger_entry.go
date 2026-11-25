package domain

import (
	"time"

	"github.com/google/uuid"
)

// LedgerEntry is an immutable, project scoped accounting fact.
type LedgerEntry struct {
	ID               uuid.UUID
	ProjectID        uuid.UUID
	EntryType        string
	AmountMicros     int64
	OperationID      *uuid.UUID
	EpisodeID        *uuid.UUID
	ShotID           *uuid.UUID
	ModelKey         *string
	Region           *string
	OrigCurrency     *string
	OrigAmountMicros *int64
	FXRate           *string
	Note             string
	CreateTime       time.Time
	CreateBy         *uuid.UUID
}
