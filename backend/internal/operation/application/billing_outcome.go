package application

import "time"

// BillingOutcome is a safe, read-only explanation. It never grants dispatch rights.
type BillingOutcome struct {
	ChargeState             string     `json:"charge_state"`
	ProviderSettlementState string     `json:"provider_settlement_state"`
	ReconciliationRequired  bool       `json:"reconciliation_required"`
	DeliveryState           string     `json:"delivery_state"`
	Regenerate              string     `json:"regenerate"`
	Reason                  string     `json:"reason"`
	ReservedMicros          *int64     `json:"reserved_micros" extensions:"x-nullable"`
	ChargedMicros           *int64     `json:"charged_micros" extensions:"x-nullable"`
	ReleasedMicros          *int64     `json:"released_micros" extensions:"x-nullable"`
	RefundedMicros          *int64     `json:"refunded_micros" extensions:"x-nullable"`
	EvidencePresent         bool       `json:"evidence_present"`
	ReconcileDeadline       *time.Time `json:"reconcile_deadline" extensions:"x-nullable"`
}

// BillingOutcomeFacts separates authoritative platform ledger from supplier
// observations. A pointer retains absent versus explicitly empty evidence.
type BillingOutcomeFacts struct {
	ReservationStatus                                                 string
	ReservedMicros, ChargedMicros, ReleasedMicros                     *int64
	ProviderSettlementState                                           *string
	ReconcileDeadline                                                 *time.Time
	Ready, Recoverable, Unrecoverable, Terminal, ReadFailed, Conflict bool
}

// ComputeBillingOutcome keeps historical debits even when supplier evidence
// disagrees. Refund completion needs an accepted compensation contract, absent here.
func ComputeBillingOutcome(f BillingOutcomeFacts) BillingOutcome {
	o := BillingOutcome{ChargeState: "unknown", ProviderSettlementState: "unknown", DeliveryState: "pending", Regenerate: "blocked", Reason: "charge_unknown", ReconciliationRequired: true, EvidencePresent: f.ProviderSettlementState != nil, ReconcileDeadline: f.ReconcileDeadline}
	if f.ReadFailed {
		return o
	}
	o.ReservedMicros, o.ChargedMicros, o.ReleasedMicros = f.ReservedMicros, f.ChargedMicros, f.ReleasedMicros
	if f.ProviderSettlementState != nil {
		switch *f.ProviderSettlementState {
		case "pending", "settled", "released", "refunded":
			o.ProviderSettlementState = *f.ProviderSettlementState
		}
	}
	switch f.ReservationStatus {
	case "held":
		o.ChargeState = "pending"
		o.Reason = "charge_pending"
	case "settled":
		if f.ChargedMicros != nil {
			o.ChargeState = "settled"
			o.ReconciliationRequired = false
			o.Reason = "charge_settled"
		}
	case "released":
		if f.ChargedMicros != nil && *f.ChargedMicros == 0 {
			o.ChargeState = "released"
			o.ReconciliationRequired = false
			o.Reason = "reservation_released"
		}
	}
	switch {
	case f.Ready:
		o.DeliveryState = "ready"
	case f.Unrecoverable:
		o.DeliveryState = "unrecoverable"
	case f.Recoverable:
		o.DeliveryState = "recoverable"
	}
	if f.Conflict || (o.ChargeState == "settled" && (o.ProviderSettlementState == "pending" || o.ProviderSettlementState == "released" || o.ProviderSettlementState == "refunded")) || (o.ChargeState == "released" && (o.ProviderSettlementState == "pending" || o.ProviderSettlementState == "settled")) {
		o.ReconciliationRequired = true
		o.Reason = "settlement_conflict"
	}
	if !o.ReconciliationRequired && f.Terminal && (o.ChargeState == "released" || f.Ready || f.Unrecoverable) {
		o.Regenerate = "confirm"
	}
	return o
}

// ConsumptionNeedsReview is only a server-side veto. Callers must read these
// facts under their admission lock and still check permissions, price and budget;
// a public billing_outcome is never an authorization input.
func ConsumptionNeedsReview(f BillingOutcomeFacts) bool {
	o := ComputeBillingOutcome(f)
	return !f.Terminal || o.ReconciliationRequired || (o.ChargeState != "settled" && o.ChargeState != "released") || (o.ChargeState != "released" && o.DeliveryState != "ready" && o.DeliveryState != "unrecoverable")
}
