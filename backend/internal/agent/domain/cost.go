package domain

import "github.com/google/uuid"

// RootCostSnapshot is the root authorization's logical capacity, reconciled
// against original call receipts and operation billing facts. It never moves funds.
type RootCostSnapshot struct {
	RootRunID         uuid.UUID          `json:"root_run_id"`
	AuthorizedLimit   int64              `json:"authorized_limit"`
	PendingCommitment int64              `json:"pending_commitment"`
	SettledCharge     int64              `json:"settled_charge"`
	AllocatedUnused   int64              `json:"allocated_unused"`
	Unallocated       int64              `json:"unallocated"`
	Remaining         int64              `json:"remaining"`
	Currency          string             `json:"currency"`
	Unit              string             `json:"unit"`
	Evidence          []RootCostEvidence `json:"evidence"`
}

// RootCostEvidence identifies the original liability and user charge. Procurement
// evidence is optional, with normalized CNY amounts and original tariff metadata.
type RootCostEvidence struct {
	ReceiptID         uuid.UUID         `json:"receipt_id"`
	OperationID       *uuid.UUID        `json:"operation_id,omitempty"`
	ReservationID     *uuid.UUID        `json:"reservation_id,omitempty"`
	SettlementID      *uuid.UUID        `json:"settlement_id,omitempty"`
	PendingCommitment int64             `json:"pending_commitment"`
	SettledCharge     int64             `json:"settled_charge"`
	Procurement       []ProcurementCost `json:"procurement,omitempty"`
}

// ProcurementCost preserves one measured supplier charge and its frozen tariff.
type ProcurementCost struct {
	ProviderCallID uuid.UUID  `json:"provider_call_id"`
	PriceVersionID *uuid.UUID `json:"price_version_id,omitempty"`
	Unit           string     `json:"unit"`
	TariffCurrency string     `json:"tariff_currency"`
	TariffUnit     string     `json:"tariff_unit"`
	FXRateToCNY    string     `json:"fx_rate_to_cny"`
	Currency       string     `json:"currency"`
	AmountMicros   int64      `json:"amount_micros"`
}
