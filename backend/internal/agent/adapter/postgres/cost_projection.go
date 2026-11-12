package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	billingdomain "github.com/StephenQiu30/glowloom-server/backend/internal/billing/domain"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

// BindOperationCostInTransaction binds the original atomic tool receipt to its
// formal operation. The deferred receipt FK permits binding in its command
// callback, before ExecuteAtomicTool writes the receipt. The unique operation
// identity prevents another call or root from accounting for the same purchase.
func BindOperationCostInTransaction(tx *gorm.DB, receiptID, operationID uuid.UUID) error {
	if tx == nil || receiptID == uuid.Nil || operationID == uuid.Nil {
		return domain.ErrInvalid
	}
	q := tx.Exec(`INSERT INTO agent_business.operation_commitment(receipt_id,operation_id) VALUES(?,?) ON CONFLICT DO NOTHING`, receiptID, operationID)
	if q.Error != nil {
		return q.Error
	}
	var count int64
	if err := tx.Raw(`SELECT count(*) FROM agent_business.operation_commitment WHERE receipt_id=? AND operation_id=?`, receiptID, operationID).Scan(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return domain.ErrConflict
	}
	return nil
}

// CostSnapshot reads the worker-authorized root cost contract. The root lock
// serializes new commitments; all financial facts are read in one SQL statement
// without row locks. Operation finalization locks operation then billing, so a
// root reader must never acquire those locks in reverse order. No notification
// is required: every snapshot and admission reconciles committed original facts.
func (s *Store) CostSnapshot(ctx context.Context, rid uuid.UUID) (domain.RootCostSnapshot, error) {
	var result domain.RootCostSnapshot
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := load(tx, rid)
		if err != nil {
			return err
		}
		if err = s.worker(tx, a); err != nil {
			return err
		}
		result, err = rootCostProjection(tx, a)
		return err
	})
	return result, err
}

type costProjectionRow struct {
	ReceiptID                                                                    uuid.UUID
	Kind                                                                         domain.CallKind
	Status                                                                       domain.CallStatus
	MaxMicros, CostMicros                                                        int64
	OperationID, ReservationID, SettlementID                                     *uuid.UUID
	OperationProjectID                                                           *uuid.UUID
	OperationStatus                                                              *operationdomain.Status
	ReservationStatus                                                            *billingdomain.ReservationStatus
	OperationSettled, AmountMicros, ChargedMicros, ReleasedMicros, ReserveMicros *int64
	Reserves, Settles, Releases                                                  int64
	Procurement                                                                  []byte
}

func rootCostProjection(tx *gorm.DB, a domain.Authority) (domain.RootCostSnapshot, error) {
	result := domain.RootCostSnapshot{RootRunID: a.RootRunID, AuthorizedLimit: a.BudgetMicros, Currency: "CNY", Unit: "micros", Evidence: []domain.RootCostEvidence{}}
	var rows []costProjectionRow
	// This single statement observes operation, reservation and ledger atomically.
	err := tx.Raw(`SELECT c.receipt_id,c.kind,c.status,c.max_micros,c.cost_micros,
 COALESCE(link.operation_id,o.id) AS operation_id,o.project_id AS operation_project_id,o.status AS operation_status,
 o.settled_micros AS operation_settled,r.id AS reservation_id,r.status AS reservation_status,r.amount_micros,
 l.reserves,l.settles,l.releases,l.reserve_micros,l.charged_micros,l.released_micros,
 settled.id AS settlement_id,purchase.procurement
 FROM agent_business.call_receipt c
 LEFT JOIN agent_business.operation_commitment link ON link.receipt_id=c.receipt_id
 LEFT JOIN operation.operation o ON o.id=COALESCE(link.operation_id,CASE WHEN c.kind=? THEN c.receipt_id END) AND NOT o.is_delete
 LEFT JOIN billing.reservation r ON r.id=o.reservation_id AND r.operation_id=o.id AND r.project_id=o.project_id AND NOT r.is_delete
 LEFT JOIN LATERAL(SELECT count(*) FILTER(WHERE entry_type='reserve') AS reserves,
 count(*) FILTER(WHERE entry_type='settle') AS settles,count(*) FILTER(WHERE entry_type='release') AS releases,
 sum(amount_micros) FILTER(WHERE entry_type='reserve') AS reserve_micros,
 sum(amount_micros) FILTER(WHERE entry_type='settle') AS charged_micros,
 sum(amount_micros) FILTER(WHERE entry_type='release') AS released_micros
 FROM billing.ledger_entry WHERE operation_id=o.id AND project_id=o.project_id AND NOT is_delete)l ON true
 LEFT JOIN billing.ledger_entry settled ON settled.operation_id=o.id AND settled.project_id=o.project_id AND settled.entry_type='settle' AND NOT settled.is_delete
 LEFT JOIN LATERAL (SELECT COALESCE(jsonb_agg(jsonb_build_object(
 'provider_call_id',p.id,'price_version_id',tariff.id,'currency','CNY','unit','micros',
 'tariff_currency',tariff.currency,'tariff_unit',tariff.unit,'fx_rate_to_cny',COALESCE(tariff.fx_rate_to_cny::text,'1'),'amount_micros',p.procurement_cost_micros)
 ORDER BY p.create_time,p.id) FILTER(WHERE p.procurement_cost_micros IS NOT NULL AND tariff.id IS NOT NULL),'[]'::jsonb) AS procurement
 FROM operation.provider_call p
 LEFT JOIN catalog.price_rule_version sale ON sale.id=p.price_rule_version_id
 LEFT JOIN catalog.price_rule_version tariff ON tariff.id::text=sale.rule->>'procurement_price_version_id'
 WHERE p.operation_id=o.id AND p.project_id=o.project_id AND NOT p.is_delete)purchase ON true
 WHERE c.root_run_id=? ORDER BY c.receipt_id`, domain.CallKindModel, a.RootRunID).Scan(&rows).Error
	if err != nil {
		return result, fmt.Errorf("read root cost facts: %w", err)
	}
	for _, r := range rows {
		evidence := domain.RootCostEvidence{ReceiptID: r.ReceiptID, OperationID: r.OperationID, ReservationID: r.ReservationID, SettlementID: r.SettlementID}
		switch {
		case r.OperationID != nil:
			var err error
			evidence, err = operationCostEvidence(a, r)
			if err != nil {
				return result, err
			}
		case r.Status == domain.CallStatusInFlight || r.Status == domain.CallStatusUnknown:
			if r.Kind == domain.CallKindModel {
				evidence.PendingCommitment = r.MaxMicros
			}
		case r.Kind == domain.CallKindTool && r.CostMicros > 0:
			// Preserve historical unbound tool commitments without inventing
			// a settlement identity from a tool name or result JSON.
			evidence.PendingCommitment = r.CostMicros
		default:
			evidence.SettledCharge = r.CostMicros
		}
		if evidence.PendingCommitment < 0 || evidence.SettledCharge < 0 || result.PendingCommitment > math.MaxInt64-evidence.PendingCommitment || result.SettledCharge > math.MaxInt64-evidence.SettledCharge {
			return result, domain.ErrUnavailable
		}
		result.PendingCommitment += evidence.PendingCommitment
		result.SettledCharge += evidence.SettledCharge
		result.Evidence = append(result.Evidence, evidence)
	}
	if result.PendingCommitment > math.MaxInt64-result.SettledCharge {
		return result, domain.ErrUnavailable
	}
	result.Remaining = max(int64(0), result.AuthorizedLimit-result.PendingCommitment-result.SettledCharge)
	result.Unallocated = result.Remaining
	return result, nil
}

func operationCostEvidence(a domain.Authority, r costProjectionRow) (domain.RootCostEvidence, error) {
	e := domain.RootCostEvidence{ReceiptID: r.ReceiptID, OperationID: r.OperationID, ReservationID: r.ReservationID, SettlementID: r.SettlementID}
	if r.OperationProjectID == nil || *r.OperationProjectID != a.Scope.ProjectID || r.ReservationStatus == nil || r.AmountMicros == nil || *r.AmountMicros < 0 || r.Reserves != 1 || r.ReserveMicros == nil || *r.ReserveMicros != *r.AmountMicros || *r.AmountMicros > r.MaxMicros {
		return e, domain.ErrUnavailable
	}
	switch r.Kind {
	case domain.CallKindTool:
		if r.Status != domain.CallStatusCompleted || r.CostMicros != 0 {
			return e, domain.ErrUnavailable
		}
	case domain.CallKindModel:
		if *r.OperationID != r.ReceiptID {
			return e, domain.ErrUnavailable
		}
	default:
		return e, domain.ErrUnavailable
	}
	if *r.ReservationStatus == billingdomain.ReservationHeld {
		if r.OperationSettled != nil || r.Settles != 0 || r.Releases != 0 || r.CostMicros != 0 {
			return e, domain.ErrUnavailable
		}
		e.PendingCommitment = *r.AmountMicros
		if r.Kind == domain.CallKindModel {
			if r.Status != domain.CallStatusInFlight && r.Status != domain.CallStatusUnknown {
				return e, domain.ErrUnavailable
			}
			// The original model authorization upper bound can exceed its wallet quote.
			e.PendingCommitment = r.MaxMicros
		}
		return e, nil
	}
	// A terminal lifecycle alone is insufficient: retain the independent financial
	// proof of exactly one original settlement and its matching release entries.
	switch {
	case r.OperationStatus == nil:
		return e, domain.ErrUnavailable
	case *r.OperationStatus != operationdomain.StatusCompleted && *r.OperationStatus != operationdomain.StatusFailed && *r.OperationStatus != operationdomain.StatusCancelled:
		return e, domain.ErrUnavailable
	case r.OperationSettled == nil || r.Settles != 1 || r.ChargedMicros == nil || *r.ChargedMicros < 0 || *r.ChargedMicros != *r.OperationSettled:
		return e, domain.ErrUnavailable
	case *r.ReservationStatus != billingdomain.ReservationSettled && *r.ReservationStatus != billingdomain.ReservationReleased:
		return e, domain.ErrUnavailable
	case *r.ReservationStatus == billingdomain.ReservationReleased && *r.ChargedMicros != 0:
		return e, domain.ErrUnavailable
	case *r.ReservationStatus == billingdomain.ReservationSettled && *r.ChargedMicros == 0:
		return e, domain.ErrUnavailable
	}
	if r.Kind == domain.CallKindModel && (r.Status != domain.CallStatusCompleted && r.Status != domain.CallStatusFailed || r.CostMicros != *r.ChargedMicros) {
		return e, domain.ErrUnavailable
	}
	released := int64(0)
	if r.ReleasedMicros != nil {
		released = *r.ReleasedMicros
	}
	expected := max(*r.AmountMicros-*r.ChargedMicros, int64(0))
	if released != expected || expected == 0 && r.Releases != 0 || expected > 0 && r.Releases != 1 {
		return e, domain.ErrUnavailable
	}
	e.SettledCharge = *r.ChargedMicros
	if len(r.Procurement) > 0 && json.Unmarshal(r.Procurement, &e.Procurement) != nil {
		return e, domain.ErrUnavailable
	}
	return e, nil
}
