package operation_test

import (
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestBillingOutcomePreservesLedgerAndUnknownEvidence(t *testing.T) {
	zero, charged := int64(0), int64(19)
	pending, empty, refunded := "pending", "", "refunded"
	past := time.Now().Add(-time.Hour)
	for _, tc := range []struct {
		name                    string
		facts                   application.BillingOutcomeFacts
		charge, provider, regen string
		reconcile               bool
	}{
		{"missing", application.BillingOutcomeFacts{}, "unknown", "unknown", "blocked", true},
		{"explicit empty", application.BillingOutcomeFacts{ProviderSettlementState: &empty}, "unknown", "unknown", "blocked", true},
		{"held beyond deadline", application.BillingOutcomeFacts{ReservationStatus: "held", ReservedMicros: &charged, ReconcileDeadline: &past}, "pending", "unknown", "blocked", true},
		{"settled upstream pending", application.BillingOutcomeFacts{ReservationStatus: "settled", ChargedMicros: &charged, ReservedMicros: &zero, ProviderSettlementState: &pending, Ready: true, Terminal: true}, "settled", "pending", "blocked", true},
		{"refund without compensation", application.BillingOutcomeFacts{ReservationStatus: "settled", ChargedMicros: &charged, ProviderSettlementState: &refunded, Ready: true, Terminal: true}, "settled", "refunded", "blocked", true},
		{"paid delivery unknown", application.BillingOutcomeFacts{ReservationStatus: "settled", ChargedMicros: &charged, Terminal: true}, "settled", "unknown", "blocked", false},
		{"paid recoverable", application.BillingOutcomeFacts{ReservationStatus: "settled", ChargedMicros: &charged, Terminal: true, Recoverable: true}, "settled", "unknown", "blocked", false},
		{"paid ready", application.BillingOutcomeFacts{ReservationStatus: "settled", ChargedMicros: &charged, Ready: true, Terminal: true}, "settled", "unknown", "confirm", false},
		{"released zero", application.BillingOutcomeFacts{ReservationStatus: "released", ChargedMicros: &zero, ReleasedMicros: &charged, Terminal: true}, "released", "unknown", "confirm", false},
		{"failed read", application.BillingOutcomeFacts{ReadFailed: true, ReservationStatus: "settled", ChargedMicros: &charged}, "unknown", "unknown", "blocked", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := application.ComputeBillingOutcome(tc.facts)
			if got.ChargeState != tc.charge || got.ProviderSettlementState != tc.provider || got.Regenerate != tc.regen || got.ReconciliationRequired != tc.reconcile {
				t.Fatalf("outcome=%+v", got)
			}
			if got.RefundedMicros != nil {
				t.Fatal("invented refund")
			}
			if tc.facts.ProviderSettlementState != nil && !got.EvidencePresent {
				t.Fatal("lost field presence")
			}
			if tc.facts.ReservationStatus == "settled" && !tc.facts.ReadFailed && (got.ChargedMicros == nil || *got.ChargedMicros != charged) {
				t.Fatal("lost historical debit")
			}
		})
	}
}
