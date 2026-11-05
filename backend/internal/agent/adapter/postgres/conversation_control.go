package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func controlReceipt(tx *gorm.DB, rid uuid.UUID, id uuid.UUID, byRequest bool) (*domain.Decision, string, *domain.AppliedReceipt, error) {
	var row struct {
		Body           []byte
		DeliveryState  string
		AppliedReceipt []byte
	}
	column := "decision_id"
	if byRequest {
		column = "request_id"
	}
	q := tx.Raw("SELECT body,delivery_state,applied_receipt FROM agent_business.control_decision WHERE root_run_id=? AND "+column+"=?", rid, id).Scan(&row)
	if q.Error != nil {
		return nil, "", nil, q.Error
	}
	if q.RowsAffected != 1 {
		return nil, "", nil, domain.ErrNotFound
	}
	var decision domain.Decision
	if err := json.Unmarshal(row.Body, &decision); err != nil {
		return nil, "", nil, domain.ErrInvalid
	}
	status := row.DeliveryState
	if status == "leased" {
		status = "pending"
	}
	var receipt *domain.AppliedReceipt
	if len(row.AppliedReceipt) > 0 {
		receipt = &domain.AppliedReceipt{}
		if json.Unmarshal(row.AppliedReceipt, receipt) != nil || receipt.DecisionID != decision.DecisionID || receipt.AuthorizationRevision != decision.AuthorizationRevision || receipt.Status != status {
			return nil, "", nil, domain.ErrInvalid
		}
	}
	if (status == "applied" || status == "superseded") && receipt == nil {
		return nil, "", nil, domain.ErrInvalid
	}
	return &decision, status, receipt, nil
}

// ReadControlStatus reads the current authorized control and delivery receipts.
func (s *Store) ReadControlStatus(ctx context.Context, actor identity.Principal, pid, sid, rid uuid.UUID, requestID *uuid.UUID) (app.ControlStatus, error) {
	var out app.ControlStatus
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := load(tx, rid)
		if err != nil {
			return err
		}
		if a.SessionID != sid || a.Scope.ActorID != actor.ID || a.Scope.ProjectID != pid || a.Scope.ScopeID != actor.ScopeID || a.Scope.OrgID != actor.OrgID {
			return domain.ErrNotFound
		}
		if err = s.request(tx, actor, pid, false); err != nil {
			return err
		}
		out.AuthorizationRevision = a.AuthorizationRevision
		if a.Current != nil {
			out.CurrentDecision, out.DeliveryStatus, out.ApplicationReceipt, err = controlReceipt(tx, rid, a.Current.DecisionID, false)
			if err != nil {
				return err
			}
		}
		if requestID != nil {
			out.RequestDecision, out.RequestDeliveryStatus, out.RequestReceipt, err = controlReceipt(tx, rid, *requestID, true)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

// UnresolvedConversationRuns lists the caller's conversation attempts still awaiting resolution.
func (s *Store) UnresolvedConversationRuns(ctx context.Context, a identity.Principal, pid, sid uuid.UUID) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.request(tx, a, pid, false); err != nil {
			return err
		}
		return tx.Raw(`SELECT DISTINCT r.root_run_id FROM agent_business.run_authorization r JOIN agent_business.call_receipt c ON c.root_run_id=r.root_run_id WHERE r.session_id=? AND r.scope->>'actor_id'=? AND r.scope->>'org_id'=? AND r.scope->>'scope_id'=? AND r.scope->>'project_id'=? AND c.status IN ('in_flight','unknown')`, sid, a.ID.String(), a.OrgID.String(), a.ScopeID.String(), pid.String()).Scan(&out).Error
	})
	return out, err
}
