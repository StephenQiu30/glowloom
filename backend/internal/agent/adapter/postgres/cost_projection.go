package postgres

import (
	"context"
	"math"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
)

// BindOperationCostInTransaction binds a tool receipt to the new AI request, never an old wallet.
func BindOperationCostInTransaction(tx *gorm.DB, receiptID, requestID uuid.UUID) error {
	if tx == nil || receiptID == uuid.Nil || requestID == uuid.Nil {
		return domain.ErrInvalid
	}
	q := tx.Exec("INSERT INTO ai.agent_intent(receipt_id,request_id,root_run_id) SELECT ?,id,run_id FROM ai.request WHERE id=? AND run_id IS NOT NULL ON CONFLICT DO NOTHING", receiptID, requestID)
	if q.Error != nil {
		return q.Error
	}
	var n int64
	if err := tx.Raw("SELECT count(*) FROM ai.agent_intent WHERE receipt_id=? AND request_id=?", receiptID, requestID).Scan(&n).Error; err != nil {
		return err
	}
	if n != 1 {
		return domain.ErrConflict
	}
	return nil
}

// CostSnapshot reads the new single ledger through Execution's public cost projection.
func (s *Store) CostSnapshot(ctx context.Context, id uuid.UUID) (domain.RootCostSnapshot, error) {
	var out domain.RootCostSnapshot
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := load(tx, id)
		if err != nil {
			return err
		}
		if err = s.worker(tx, a); err != nil {
			return err
		}
		out, err = rootCostProjection(tx, a)
		return err
	})
	return out, err
}
func rootCostProjection(tx *gorm.DB, a domain.Authority) (domain.RootCostSnapshot, error) {
	out := domain.RootCostSnapshot{RootRunID: a.RootRunID, AuthorizedLimit: a.BudgetMicros, Currency: "CNY", Unit: "micros", Evidence: []domain.RootCostEvidence{}}
	var rows []struct {
		ReceiptID                                                   uuid.UUID
		Kind, Status                                                string
		MaxMicros, CostMicros                                       int64
		RequestID, ProjectID, ScopeID, ActorID, RunID, SettlementID *uuid.UUID
		AIState                                                     *string
		HeldMicros, ChargedMicros, ReservedMicros                   *int64
	}
	err := tx.Raw(`SELECT c.receipt_id,c.kind,c.status,c.max_micros,c.cost_micros,v.request_id,v.project_id,v.scope_id,v.actor_id,v.run_id,v.settlement_id,v.state AS ai_state,v.held_micros,v.charged_micros,v.reserved_micros
 FROM agent_business.call_receipt c LEFT JOIN ai.call_cost v ON (v.receipt_id=c.receipt_id OR (c.kind='model' AND v.request_id=c.receipt_id)) AND v.run_id=c.root_run_id
 WHERE c.root_run_id=? ORDER BY c.receipt_id`, a.RootRunID).Scan(&rows).Error
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		e := domain.RootCostEvidence{ReceiptID: row.ReceiptID}
		switch {
		case row.RequestID != nil:
			if row.ProjectID == nil || *row.ProjectID != a.Scope.ProjectID || row.ScopeID == nil || *row.ScopeID != a.Scope.ScopeID || row.ActorID == nil || *row.ActorID != a.Scope.ActorID || row.RunID == nil || *row.RunID != a.RootRunID || row.AIState == nil || row.HeldMicros == nil || row.ChargedMicros == nil || row.ReservedMicros == nil || *row.ReservedMicros > row.MaxMicros {
				return out, domain.ErrUnavailable
			}
			e.OperationID = row.RequestID
			e.ReservationID = row.RequestID
			e.SettlementID = row.SettlementID
			e.PendingCommitment = *row.HeldMicros
			e.SettledCharge = *row.ChargedMicros
		case row.Kind == domain.CallKindModel && (row.Status == domain.CallStatusInFlight || row.Status == domain.CallStatusUnknown):
			e.PendingCommitment = row.MaxMicros
		case row.CostMicros != 0:
			return out, domain.ErrUnavailable
		}
		if e.PendingCommitment < 0 || e.SettledCharge < 0 || out.PendingCommitment > math.MaxInt64-e.PendingCommitment || out.SettledCharge > math.MaxInt64-e.SettledCharge {
			return out, domain.ErrUnavailable
		}
		out.PendingCommitment += e.PendingCommitment
		out.SettledCharge += e.SettledCharge
		out.Evidence = append(out.Evidence, e)
	}
	if out.PendingCommitment > math.MaxInt64-out.SettledCharge {
		return out, domain.ErrUnavailable
	}
	out.Remaining = max(int64(0), out.AuthorizedLimit-out.PendingCommitment-out.SettledCharge)
	out.Unallocated = out.Remaining
	return out, nil
}
