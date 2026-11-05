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
