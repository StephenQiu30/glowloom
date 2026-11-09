package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

type taskEvidenceOperationRow struct {
	ID      uuid.UUID
	BatchID *uuid.UUID
}

type taskEvidenceProviderCallRow struct {
	ResponseSummary   []byte
	ProviderTaskID    *string
	DispatchStartedAt *time.Time
}

// ScheduleOperationStep admits a single execution kind for an existing operation.
// It is safe to repeat after a crash between a transition and scheduling.
func (s *Store) ScheduleOperationStep(ctx context.Context, kind string, id uuid.UUID) error {
	return taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error { return taskruntime.Enqueue(ctx, tx, kind, id.String(), time.Now()) })
}

// TaskEvidence reconstructs private submission and media progress from the
// original business ledgers. No provider payload enters the task runtime.
func (s *Store) TaskEvidence(ctx context.Context, id uuid.UUID) (application.TaskEvidence, error) {
	var evidence application.TaskEvidence
	if s == nil || s.db == nil || id == uuid.Nil {
		return evidence, ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var operation taskEvidenceOperationRow
		if err := tx.Table("operation.operation").Select("id", "batch_id").Where("id = ? AND NOT is_delete", id).Take(&operation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("read operation task evidence identity: %w", err)
		}

		var cancelEvents int64
		if err := tx.Table("operation.operation_event").Where("operation_id = ? AND reason = ? AND NOT is_delete", id, "user_cancel_requested").Count(&cancelEvents).Error; err != nil {
			return fmt.Errorf("read operation cancellation evidence: %w", err)
		}
		evidence.CancelRequested = cancelEvents > 0
		if operation.BatchID == nil {
			evidence.Admitted = true
		} else {
			var batch struct {
				CancelRequestedAt *time.Time
			}
			err := tx.Table("operation.batch").Select("cancel_requested_at").Where("id = ?", *operation.BatchID).Take(&batch).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("read operation batch cancellation evidence: %w", err)
			}
			evidence.CancelRequested = evidence.CancelRequested || batch.CancelRequestedAt != nil
			var activeLaunches int64
			if err := tx.Table("operation.batch_launch").Where("operation_id = ? AND state = ?", id, domain.BatchLaunchStateActive).Count(&activeLaunches).Error; err != nil {
				return fmt.Errorf("read operation batch admission evidence: %w", err)
			}
			evidence.Admitted = activeLaunches > 0
		}

		var submit taskEvidenceProviderCallRow
		err := tx.Table("operation.provider_call").Select("response_summary", "provider_task_id", "dispatch_started_at").
			Where("operation_id = ? AND action = ? AND attempt = ? AND NOT is_delete", id, "submit", 1).Take(&submit).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read operation submit evidence: %w", err)
		}
		if err == nil {
			var summary struct {
				DispatchCancelled bool                          `json:"dispatch_cancelled"`
				FailureCode       string                        `json:"failure_code"`
				State             application.ProviderCallState `json:"state"`
			}
			if len(submit.ResponseSummary) > 0 && json.Unmarshal(submit.ResponseSummary, &summary) != nil {
				return application.ErrInvalidProviderCall
			}
			evidence.DispatchCancelled = summary.DispatchCancelled
			evidence.SubmitState = summary.State
			evidence.FailureCode = summary.FailureCode
			evidence.SubmitTaskID = submit.ProviderTaskID
			evidence.DispatchStartedAt = submit.DispatchStartedAt
		}

		var cancel taskEvidenceProviderCallRow
		err = tx.Table("operation.provider_call").Select("response_summary").
			Where("operation_id = ? AND action = ? AND NOT is_delete", id, "cancel").Order("attempt DESC").Take(&cancel).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read operation cancel evidence: %w", err)
		}
		if err == nil && len(cancel.ResponseSummary) > 0 {
			var summary struct {
				State application.ProviderCallState `json:"state"`
			}
			if json.Unmarshal(cancel.ResponseSummary, &summary) != nil {
				return application.ErrInvalidProviderCall
			}
			evidence.CancelState = summary.State
		}

		var query taskEvidenceProviderCallRow
		err = tx.Table("operation.provider_call").Select("response_summary").
			Where("operation_id = ? AND action = ? AND NOT is_delete", id, "query").Order("attempt DESC").Take(&query).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read operation query evidence: %w", err)
		}
		if err == nil && len(query.ResponseSummary) > 0 {
			var summary struct {
				FailureCode string                        `json:"failure_code"`
				State       application.ProviderCallState `json:"state"`
			}
			if json.Unmarshal(query.ResponseSummary, &summary) != nil {
				return application.ErrInvalidProviderCall
			}
			evidence.QueryState = summary.State
			evidence.QueryFailureCode = summary.FailureCode
		}

		var completed, failed int64
		if err := tx.Table("infra.task_work").Where("kind = ? AND business_id = ? AND completed_at IS NOT NULL", "operation.ingest", id.String()).Count(&completed).Error; err != nil {
			return fmt.Errorf("read operation ingest completion: %w", err)
		}
		if err := tx.Table("infra.task_work").Where("kind = ? AND business_id = ? AND failed_at IS NOT NULL", "operation.ingest", id.String()).Count(&failed).Error; err != nil {
			return fmt.Errorf("read operation ingest failure: %w", err)
		}
		evidence.Ingested = completed > 0
		evidence.IngestFailed = failed > 0
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return evidence, err
}

// ScheduleOriginalDelivery enqueues delivery of the already admitted original result.
func (s *Store) ScheduleOriginalDelivery(ctx context.Context, d application.WorkflowControlDelivery) error {
	return taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		return taskruntime.EnqueueReference(ctx, tx, "operation.original", d.EventID.String(), time.Now(), d)
	})
}

// RecoverOperationTasks consumes one explicit recovery intent. Only parked reads
// or delivery of the original dispatched result can be reopened; admission never
// resets a submit claim, completed work or an active execution window.
func (s *Store) RecoverOperationTasks(ctx context.Context, d application.WorkflowControlDelivery) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if d.TargetType != "operation" || d.Action != "recover" {
		return application.ErrInvalidWorkflowControl
	}
	return taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		if err := requireControlReceipt(tx, d); err != nil {
			return err
		}
		if err := s.requireRecoveryRequester(tx, d); err != nil {
			return err
		}
		var op struct {
			Status     domain.Status
			AdapterKey string
		}
		r := tx.Raw(`SELECT o.status,p.adapter_key FROM operation.operation o
   JOIN catalog.model_profile_version v ON v.id=o.model_profile_version_id
   JOIN catalog.model_profile m ON m.id=v.model_profile_id
   JOIN catalog.provider p ON p.id=m.provider_id
   WHERE o.id=? AND o.project_id=? AND NOT o.is_delete FOR UPDATE OF o`, d.TargetID, d.ProjectID).Scan(&op)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrWorkflowControlConflict
		}
		// The operation lock serializes concurrent deliveries. This existing event
		// ledger keeps the consumed intent after HTTP idempotency retention expires.
		appliedID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("glowloom.operation.recovery/"+d.EventID.String()))
		r = tx.Exec(`INSERT INTO operation.operation_event(id,operation_id,from_status,to_status,reason,detail)
   VALUES(?,?,?,?, 'user_recover_applied',jsonb_build_object('event_id',?::text)) ON CONFLICT(id) DO NOTHING`,
			appliedID, d.TargetID, op.Status, op.Status, d.EventID.String())
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return nil
		}
		kinds := []string{"operation.run"}
		switch op.Status {
		case domain.StatusSubmitted, domain.StatusReconciling, domain.StatusManual:
			kinds = append(kinds, "operation.provider")
		case domain.StatusSucceeded, domain.StatusIngesting:
			kinds = append(kinds, "operation.ingest")
		default:
			return nil
		}
		if !catalogdomain.IsManagedCloud(op.AdapterKey) {
			return nil
		}
		var safe bool
		if err := tx.Raw(`SELECT EXISTS (
   SELECT 1 FROM operation.operation o
   JOIN catalog.model_profile_version v ON v.id=o.model_profile_version_id
   JOIN operation.provider_call c ON c.operation_id=o.id AND c.project_id=o.project_id
    AND c.action='submit' AND c.attempt=1 AND NOT c.is_delete
   JOIN operation.provider_result pr ON pr.operation_id=o.id
   WHERE o.id=? AND c.dispatch_started_at IS NOT NULL AND c.request_summary->>'dispatch_contract'='v1'
    AND c.request_key=o.provider_request_key AND c.model_profile_version_id=o.model_profile_version_id
    AND c.price_rule_version_id=o.price_rule_version_id
    AND pr.result->'identity'->>'operation_id'=o.id::text
    AND pr.result->'identity'->>'project_id'=o.project_id::text
    AND pr.result->'identity'->>'request_key'=o.provider_request_key
    AND pr.result->'identity'->>'model_profile_version_id'=o.model_profile_version_id::text
    AND pr.result->'identity'->>'price_rule_version_id'=o.price_rule_version_id::text
    AND pr.result->'identity'->>'action'='submit' AND pr.result->'identity'->>'attempt'='1'
    AND ((o.status IN ('submitted','reconciling','manual') AND v.supports_query
      AND pr.result->>'state' IN ('accepted','succeeded') AND c.provider_task_id<>''
      AND pr.result->>'task_id'=c.provider_task_id
      AND c.provider_task_id=(SELECT e.detail->>'provider_task_id' FROM operation.operation_event e
       WHERE e.operation_id=o.id AND e.detail ? 'provider_task_id' AND NOT e.is_delete
       ORDER BY e.create_time DESC,e.id DESC LIMIT 1))
     OR (o.status IN ('succeeded','ingesting') AND pr.result->>'state'='succeeded'))
  )`, d.TargetID).Row().Scan(&safe); err != nil {
			return fmt.Errorf("verify original recovery dispatch: %w", err)
		}
		if safe {
			if err := tx.Exec(`UPDATE infra.task_work SET failed_at=NULL,failures=0,next_run_at=now(),
    owner=NULL,lease_until=NULL,epoch=epoch+1
    WHERE business_id=? AND kind IN ? AND failed_at IS NOT NULL AND completed_at IS NULL
    AND stop_requested_at IS NULL AND (lease_until IS NULL OR lease_until<=now())`, d.TargetID.String(), kinds).Error; err != nil {
				return fmt.Errorf("reopen original operation reads: %w", err)
			}
		}
		return taskruntime.Enqueue(ctx, tx, "operation.run", d.TargetID.String(), time.Now())
	})
}
