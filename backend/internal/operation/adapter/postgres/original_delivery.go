package postgres

import (
	"context"
	"errors"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// originalDeliveryAllowed requires immutable paid evidence, an exact original
// dispatch identity and no rejected output. It never authorizes a provider call.
func originalDeliveryAllowed(tx *gorm.DB, id uuid.UUID) (bool, error) {
	var row struct {
		ProjectID uuid.UUID
		Status    domain.Status
	}
	read := tx.Raw(`SELECT o.project_id,o.status FROM operation.operation o
 JOIN operation.provider_result pr ON pr.operation_id=o.id AND pr.result->>'state'='succeeded'
 JOIN catalog.model_profile_version v ON v.id=o.model_profile_version_id AND v.moderation='provider'
 WHERE o.id=? AND o.status IN ('failed','cancelled','completed') AND NOT o.is_delete
 AND EXISTS(SELECT 1 FROM operation.provider_call c WHERE c.operation_id=o.id AND c.project_id=o.project_id AND c.action='submit' AND c.attempt=1 AND c.dispatch_started_at IS NOT NULL AND c.request_key=o.provider_request_key AND c.model_profile_version_id=o.model_profile_version_id AND c.price_rule_version_id=o.price_rule_version_id AND c.request_summary->>'dispatch_contract'='v1' AND NOT c.is_delete)
 AND NOT EXISTS(SELECT 1 FROM operation.operation_output ot WHERE ot.operation_id=o.id AND ot.project_id=o.project_id AND (ot.is_delete OR ot.moderation_status='rejected'))`, id).Scan(&row)
	if read.Error != nil || read.RowsAffected != 1 {
		return false, read.Error
	}
	// readBillingFacts validates the exact result identity and the immutable
	// reserve/settle/release set using the latest supplier observation.
	result, err := readBillingOutcome(tx, row.ProjectID, id, row.Status)
	return result.ChargeState == "settled" && !result.ReconciliationRequired && result.ChargedMicros != nil && *result.ChargedMicros > 0, err
}

// OriginalDeliveryRecoveryRequired distinguishes a terminal archive from a
// signal to an existing execution. Only the archive uses the recovery actor.
func (s *Store) OriginalDeliveryRecoveryRequired(ctx context.Context, d application.WorkflowControlDelivery) (bool, error) {
	if s == nil || s.db == nil {
		return false, ErrUnavailable
	}
	var terminal bool
	err := taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		var status domain.Status
		if err := tx.Raw(`SELECT status FROM operation.operation WHERE id=? AND project_id=? AND NOT is_delete`, d.TargetID, d.ProjectID).Scan(&status).Error; err != nil {
			return err
		}
		terminal = status.IsTerminal()
		if !terminal {
			return s.requireWorker(tx, d.TargetID, true)
		}
		return s.CheckOriginalDelivery(tx, d)
	})
	return terminal, err
}

// CheckOriginalDelivery binds a terminal archive to its accepted request and
// rechecks the actual requester, private target and settlement within the writer's transaction.
func (s *Store) CheckOriginalDelivery(tx *gorm.DB, d application.WorkflowControlDelivery) error {
	if d.TargetType != "operation" || d.Action != "recover" {
		return application.ErrInvalidWorkflowControl
	}
	if err := s.requireRecoveryRequester(tx, d); err != nil {
		return err
	}
	var present int
	locked := tx.Raw(`SELECT 1 FROM operation.operation WHERE id=? AND project_id=? AND NOT is_delete FOR UPDATE`, d.TargetID, d.ProjectID).Scan(&present)
	if locked.Error != nil {
		return locked.Error
	}
	if locked.RowsAffected != 1 {
		return application.ErrWorkflowControlConflict
	}
	allowed, err := originalDeliveryAllowed(tx, d.TargetID)
	if err != nil {
		return err
	}
	if !allowed {
		return application.ErrWorkflowControlConflict
	}
	return nil
}

// The event is the durable accepted intent, independent of HTTP idempotency
// record retention. Never reconstruct a session for a legacy unbound intent.
func (s *Store) requireRecoveryRequester(tx *gorm.DB, d application.WorkflowControlDelivery) error {
	var actor identity.Principal
	r := tx.Raw(`SELECT (detail->>'session_id')::uuid AS session_id,(detail->>'scope_id')::uuid AS scope_id,(detail->>'credential_epoch')::bigint AS credential_epoch
 FROM operation.operation_event WHERE operation_id=? AND reason='user_recover_requested' AND detail->>'event_id'=? AND detail->>'request_id'=? AND detail->>'actor_id'=? AND NOT is_delete`, d.TargetID, d.EventID.String(), d.RequestID.String(), d.ActorID.String()).Scan(&actor)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return application.ErrInvalidWorkflowControl
	}
	actor.ID, actor.OrgID = d.ActorID, d.OrgID
	if err := s.requirePublicProject(tx, actor, d.ProjectID, true); err != nil {
		if errors.Is(err, identity.ErrForbidden) || errors.Is(err, identity.ErrSessionInvalid) || errors.Is(err, workspace.ErrScopeNotFound) || errors.Is(err, workspace.ErrProjectNotFound) || errors.Is(err, application.ErrPublicNotFound) {
			return application.ErrExecutionAuthorityRevoked
		}
		return err
	}
	if err := requireAgentOperationOwner(tx, actor, d.ProjectID, d.TargetID); err != nil {
		if errors.Is(err, application.ErrPublicNotFound) {
			return application.ErrExecutionAuthorityRevoked
		}
		return err
	}
	return nil
}

// OriginalDeliveryIdentity returns the frozen identity only after the same
// request-bound checks used by each archive writer.
func (s *Store) OriginalDeliveryIdentity(ctx context.Context, d application.WorkflowControlDelivery) (application.ProviderDispatchIdentity, error) {
	var i application.ProviderDispatchIdentity
	err := taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		if err := s.CheckOriginalDelivery(tx, d); err != nil {
			return err
		}
		return tx.Raw(`SELECT project_id,id AS operation_id,model_profile_version_id,price_rule_version_id,provider_request_key AS request_key,1 AS attempt,'submit' AS action FROM operation.operation WHERE id=?`, d.TargetID).Scan(&i).Error
	})
	return i, err
}
