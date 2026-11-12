package postgres

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

// CanDeliverWorkflowControl verifies the event's committed receipt and live target.
// The committed receipt is validated before current authority; terminal tasks are no-ops.
func (s *Store) canDeliverWorkflowControl(ctx context.Context, delivery application.WorkflowControlDelivery) (bool, error) {
	if s == nil || s.db == nil {
		return false, ErrUnavailable
	}
	if delivery.TargetType != application.WorkflowTargetOperation && delivery.TargetType != application.WorkflowTargetBatch {
		return false, application.ErrInvalidWorkflowControl
	}
	if err := requireControlReceipt(s.db.WithContext(ctx), delivery); err != nil {
		return false, err
	}
	var status string
	var readErr error
	if delivery.TargetType == application.WorkflowTargetOperation {
		readErr = s.db.WithContext(ctx).Raw(`SELECT status FROM operation.operation WHERE id=?::uuid AND project_id=?::uuid AND NOT is_delete`, delivery.TargetID, delivery.ProjectID).Scan(&status).Error
		if readErr == nil && delivery.Action == application.WorkflowActionRecover && domain.Status(status).IsTerminal() {
			outcome, err := readBillingOutcome(s.db.WithContext(ctx), delivery.ProjectID, delivery.TargetID, domain.Status(status))
			if err != nil || outcome.DeliveryState == "ready" {
				return false, err
			}
			if err := s.requireRecoveryRequester(s.db.WithContext(ctx), delivery); err != nil {
				return false, err
			}
			return originalDeliveryAllowed(s.db.WithContext(ctx), delivery.TargetID)
		}
		if readErr == nil && (status == "" || domain.Status(status).IsTerminal()) {
			return false, nil
		}
	} else {
		readErr = s.db.WithContext(ctx).Raw(`SELECT status FROM operation.batch WHERE id=?::uuid AND project_id=?::uuid AND NOT is_delete`, delivery.TargetID, delivery.ProjectID).Scan(&status).Error
		if readErr == nil && status != string(domain.BatchStatusConfirmed) && status != string(domain.BatchStatusRunning) {
			return false, nil
		}
	}
	if readErr != nil {
		return false, fmt.Errorf("read workflow control target: %w", readErr)
	}
	return true, nil
}

func requireControlReceipt(tx *gorm.DB, delivery application.WorkflowControlDelivery) error {
	var receipt bool
	err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM infra.idempotency_record AS r
 JOIN workspace.project AS p ON p.id=?::uuid AND p.org_id=?::uuid
 WHERE r.actor_id=?::uuid AND r.idem_key=? AND NOT r.is_delete AND r.status_code=202
 AND r.response_body->>'event_id'=? AND r.response_body->>'target_id'=?
 AND r.response_body->>'target_type'=? AND r.response_body->>'action'=?
 AND r.response_body->>'accepted'='true')`, delivery.ProjectID, delivery.OrgID, delivery.ActorID, delivery.RequestID.String(), delivery.EventID.String(), delivery.TargetID.String(), delivery.TargetType, delivery.Action).Scan(&receipt).Error
	if err != nil {
		return fmt.Errorf("read workflow control binding: %w", err)
	}
	if !receipt {
		return application.ErrInvalidWorkflowControl
	}
	return nil
}

func (s *Store) requireControlActor(tx *gorm.DB, d application.WorkflowControlDelivery) error {
	if s.workerAccess == nil || s.workerAccess(tx) == nil {
		return identity.ErrForbidden
	}
	actor := identity.Principal{ID: d.ActorID, OrgID: d.OrgID}
	if err := s.workerAccess(tx).AuthorizeOperation(tx.Statement.Context, actor, d.ProjectID, true); err != nil {
		return err
	}
	if d.TargetType == application.WorkflowTargetOperation {
		return requireAgentOperationOwner(tx, actor, d.ProjectID, d.TargetID)
	}
	return nil
}

// CanDeliverWorkflowControl rechecks the durable control actor before delivering a signal.
func (s *Store) CanDeliverWorkflowControl(ctx context.Context, delivery application.WorkflowControlDelivery) (bool, error) {
	if s == nil || s.db == nil {
		return false, ErrUnavailable
	}
	var allowed bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		allowed, err = s.withDB(tx).canDeliverWorkflowControl(ctx, delivery)
		if err != nil || !allowed {
			return err
		}
		// Recovery rechecks the request session again when applying the intent.
		// Take that account/session lock before worker authorization locks the
		// project. Upgrading a shared account lock while retaining the project
		// lock deadlocks with a concurrent delivery waiting on that project.
		if delivery.TargetType == application.WorkflowTargetOperation && delivery.Action == application.WorkflowActionRecover {
			if err := s.requireRecoveryRequester(tx, delivery); err != nil {
				return err
			}
		}
		return s.requireControlActor(tx, delivery)
	})
	return allowed, err
}
