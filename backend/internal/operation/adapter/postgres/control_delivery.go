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
	if delivery.TargetType != "operation" && delivery.TargetType != "batch" {
		return false, application.ErrInvalidWorkflowControl
	}
	var receipt bool
	err := s.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM infra.idempotency_record AS r
 JOIN workspace.project AS p ON p.id=?::uuid AND p.org_id=?::uuid
 WHERE r.actor_id=?::uuid AND r.idem_key=? AND NOT r.is_delete AND r.status_code=202
 AND r.response_body->>'event_id'=? AND r.response_body->>'target_id'=?
 AND r.response_body->>'target_type'=? AND r.response_body->>'action'=?
 AND r.response_body->>'accepted'='true')`, delivery.ProjectID, delivery.OrgID, delivery.ActorID, delivery.RequestID.String(), delivery.EventID.String(), delivery.TargetID.String(), delivery.TargetType, delivery.Action).Scan(&receipt).Error
	if err != nil {
		return false, fmt.Errorf("read workflow control binding: %w", err)
	}
	if !receipt {
		return false, application.ErrInvalidWorkflowControl
	}
	var status string
	var readErr error
	if delivery.TargetType == "operation" {
		readErr = s.db.WithContext(ctx).Raw(`SELECT status FROM operation.operation WHERE id=?::uuid AND project_id=?::uuid AND NOT is_delete`, delivery.TargetID, delivery.ProjectID).Scan(&status).Error
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
		if s.workerAccess == nil || s.workerAccess(tx) == nil {
			return identity.ErrForbidden
		}
		if err := s.workerAccess(tx).AuthorizeOperation(ctx, identity.Principal{ID: delivery.ActorID, OrgID: delivery.OrgID}, delivery.ProjectID, true); err != nil {
			return err
		}
		return nil
	})
	return allowed, err
}
