package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

const (
	ReservationReserved = "reserved"
	ReservationSettled  = "settled"
	ReservationReleased = "released"

	LedgerReservationCreated  = "reservation_created"
	LedgerReservationSettled  = "reservation_settled"
	LedgerReservationReleased = "reservation_released"
)

type Reservation struct {
	ID, WorkspaceID, ProjectID                             string
	EstimateID, BudgetPolicyID, PriceQuoteID               string
	Metric, SourceType, SourceID                           string
	EstimatedUnits, SettledUnits                           int64
	UnitAmount, ReservedAmount, SettledAmount, BudgetLimit decimal.Decimal
	Currency                                               string
	PriceQuoteRevision, BudgetPolicyRevision, Revision     int64
	Status, ContentHash, CreatedBy, UpdatedBy              string
	UsageReceiptID                                         *string
	SettledAt, ReleasedAt                                  *time.Time
	CreatedAt, UpdatedAt                                   time.Time
}

func SameReservationBinding(left, right Reservation) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.EstimateID == right.EstimateID && left.BudgetPolicyID == right.BudgetPolicyID &&
		left.PriceQuoteID == right.PriceQuoteID && left.Metric == right.Metric &&
		left.SourceType == right.SourceType && left.SourceID == right.SourceID &&
		left.EstimatedUnits == right.EstimatedUnits && left.UnitAmount.Equal(right.UnitAmount) &&
		left.ReservedAmount.Equal(right.ReservedAmount) && left.Currency == right.Currency &&
		left.PriceQuoteRevision == right.PriceQuoteRevision && left.BudgetPolicyRevision == right.BudgetPolicyRevision &&
		left.BudgetLimit.Equal(right.BudgetLimit)
}

func SameReservationState(left, right Reservation) bool {
	return left.ID == right.ID && SameReservationBinding(left, right) && left.SettledUnits == right.SettledUnits &&
		left.SettledAmount.Equal(right.SettledAmount) && left.Status == right.Status &&
		optionalStringEqual(left.UsageReceiptID, right.UsageReceiptID) && left.Revision == right.Revision &&
		left.ContentHash == right.ContentHash
}

type LedgerEntry struct {
	ID, WorkspaceID, ProjectID, ReservationID, EstimateID string
	EntryType                                             string
	Sequence                                              int64
	ReservedDelta, SettledDelta                           decimal.Decimal
	Currency, ContentHash, CreatedBy                      string
	UsageReceiptID                                        *string
	CreatedAt                                             time.Time
}

func SameLedgerEntryState(left, right LedgerEntry) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.ReservationID == right.ReservationID && left.EstimateID == right.EstimateID &&
		left.EntryType == right.EntryType && left.Sequence == right.Sequence &&
		left.ReservedDelta.Equal(right.ReservedDelta) && left.SettledDelta.Equal(right.SettledDelta) &&
		left.Currency == right.Currency && optionalStringEqual(left.UsageReceiptID, right.UsageReceiptID) &&
		left.ContentHash == right.ContentHash
}

func optionalStringEqual(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
