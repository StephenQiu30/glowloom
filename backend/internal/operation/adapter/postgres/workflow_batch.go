package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

type batchFinishedEvent struct {
	EventID    uuid.UUID `json:"event_id"`
	EventType  string    `json:"event_type"`
	OccurredAt time.Time `json:"occurred_at"`
	OrgID      uuid.UUID `json:"org_id"`
	ProjectID  uuid.UUID `json:"project_id"`
	Actor      struct {
		Kind string `json:"kind"`
	} `json:"actor"`
	Aggregate struct {
		Type string    `json:"type"`
		ID   uuid.UUID `json:"id"`
	} `json:"aggregate"`
	Data struct {
		BatchID        uuid.UUID `json:"batch_id"`
		Kind           string    `json:"kind"`
		Status         string    `json:"status"`
		TotalCount     int32     `json:"total_count"`
		SucceededCount int32     `json:"succeeded_count"`
		FailedCount    int32     `json:"failed_count"`
		UnknownCount   int32     `json:"unknown_count"`
		CancelledCount int32     `json:"cancelled_count"`
	} `json:"data"`
}

// LoadWorkflowBatch rechecks the confirmed selection on every parent run.
// Expired or excluded quoted members retain batch_id but are never executable.
func (s *Store) LoadWorkflowBatch(ctx context.Context, batchID uuid.UUID) (application.WorkflowBatch, error) {
	if s == nil || s.db == nil {
		return application.WorkflowBatch{}, ErrUnavailable
	}
	if batchID == uuid.Nil {
		return application.WorkflowBatch{}, application.ErrInvalidWorkflowBatch
	}
	var snapshot application.WorkflowBatch
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row batchRow
		result := tx.Raw(`
			SELECT b.id, b.project_id, b.kind, b.scope, b.status,
			       b.total_count, b.succeeded_count, b.failed_count,
		       b.unknown_count, b.quote_total_micros, b.paused_reason, b.cancel_requested_at
			FROM operation.batch AS b
			JOIN workspace.project AS p ON p.id = b.project_id
			WHERE b.id = ?::uuid AND NOT b.is_delete AND NOT p.is_delete
			FOR SHARE OF b
		`, batchID.String()).Scan(&row)
		if result.Error != nil {
			return fmt.Errorf("read workflow batch: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		batch := row.domain()
		if err := batch.Validate(); err != nil || batch.TotalCount > 300 ||
			(batch.Status != domain.BatchStatusConfirmed && batch.Status != domain.BatchStatusRunning &&
				batch.Status != domain.BatchStatusFinished && batch.Status != domain.BatchStatusCancelled) {
			return application.ErrInvalidWorkflowBatch
		}
		snapshot.Batch = batch
		if row.PausedReason != nil {
			snapshot.PausedReason = *row.PausedReason
		}
		snapshot.CancelRequested = row.CancelRequestedAt != nil
		var items []struct {
			ID            uuid.UUID
			Status        string
			FailureCode   *string
			FinishedAt    *time.Time
			ReservationID *uuid.UUID
			WorkflowID    *string
			TargetType    *string
		}
		result = tx.Raw(`
			SELECT id, status, failure_code, finished_at, reservation_id, workflow_id, target_type
			FROM operation.operation
			WHERE batch_id = ?::uuid AND project_id = ?::uuid AND NOT is_delete
			  AND status <> 'expired'
			ORDER BY id
		`, batchID.String(), batch.ProjectID.String()).Scan(&items)
		if result.Error != nil {
			return fmt.Errorf("read workflow batch items: %w", result.Error)
		}
		if len(items) != int(batch.TotalCount) {
			return application.ErrInvalidWorkflowBatch
		}
		snapshot.Items = make([]application.WorkflowBatchItem, 0, len(items))
		for _, item := range items {
			status := domain.Status(item.Status)
			if item.ID == uuid.Nil || item.ReservationID == nil || *item.ReservationID == uuid.Nil ||
				item.WorkflowID == nil || *item.WorkflowID != "operation/"+item.ID.String() ||
				item.TargetType == nil || *item.TargetType == "agent_session" ||
				status == domain.StatusDraft || status == domain.StatusQuoted || status == domain.StatusExpired ||
				status.CanTransitionTo(status) != nil {
				return application.ErrInvalidWorkflowBatch
			}
			selected := application.WorkflowBatchItem{OperationID: item.ID, Status: status}
			if item.FailureCode != nil {
				selected.FailureCode = *item.FailureCode
			}
			if item.FinishedAt != nil {
				selected.FinishedAt = *item.FinishedAt
			}
			snapshot.Items = append(snapshot.Items, selected)
		}
		return nil
	})
	if err != nil {
		return application.WorkflowBatch{}, fmt.Errorf("load workflow batch: %w", err)
	}
	return snapshot, nil
}

// MarkWorkflowBatchRunning conditionally records that the parent owns the
// confirmed selection. Repeating the transition after an Activity retry is safe.
func (s *Store) MarkWorkflowBatchRunning(ctx context.Context, batchID uuid.UUID) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if batchID == uuid.Nil {
		return application.ErrInvalidWorkflowBatch
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(`
			UPDATE operation.batch AS b
			SET status = 'running', update_time = now()
			WHERE b.id = ?::uuid AND b.status = 'confirmed' AND NOT b.is_delete
			  AND b.total_count BETWEEN 1 AND 300
			  AND b.workflow_id = 'batch/' || b.id::text
			  AND EXISTS (
			    SELECT 1 FROM workspace.project AS p
			    WHERE p.id = b.project_id AND NOT p.is_delete
			  )
			  AND b.total_count = (
			    SELECT count(*) FROM operation.operation AS o
			    WHERE o.batch_id = b.id AND o.project_id = b.project_id AND NOT o.is_delete
			      AND o.status = 'confirmed' AND o.reservation_id IS NOT NULL
			      AND o.workflow_id = 'operation/' || o.id::text
			      AND o.target_type IS DISTINCT FROM 'agent_session'
			  )
			  AND NOT EXISTS (
			    SELECT 1 FROM operation.operation AS o
			    WHERE o.batch_id = b.id AND o.project_id = b.project_id AND NOT o.is_delete
			      AND o.status NOT IN ('confirmed', 'expired')
			  )
		`, batchID.String())
		if result.Error != nil {
			return fmt.Errorf("mark workflow batch running: %w", result.Error)
		}
		if result.RowsAffected == 1 {
			return nil
		}
		var row struct{ Status string }
		read := tx.Raw(`SELECT status FROM operation.batch WHERE id = ?::uuid AND NOT is_delete`,
			batchID.String()).Scan(&row)
		if read.Error != nil {
			return fmt.Errorf("read workflow batch state: %w", read.Error)
		}
		if read.RowsAffected == 1 && row.Status == string(domain.BatchStatusRunning) {
			return nil
		}
		return application.ErrInvalidWorkflowBatch
	})
}

// PauseWorkflowBatch blocks further child admission after the parent observes
// five consecutive failures with the same normalized provider code.
func (s *Store) PauseWorkflowBatch(ctx context.Context, batchID uuid.UUID, reason string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if batchID == uuid.Nil || !strings.HasPrefix(reason, "provider:") ||
		len(reason) <= len("provider:") || len(reason) > len("provider:")+64 {
		return application.ErrInvalidWorkflowBatch
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updated := tx.Exec(`
			UPDATE operation.batch SET paused_reason = ?, update_time = now()
			WHERE id = ?::uuid AND status = 'running' AND paused_reason IS NULL AND NOT is_delete
		`, reason, batchID.String())
		if updated.Error != nil {
			return fmt.Errorf("pause workflow batch: %w", updated.Error)
		}
		if updated.RowsAffected == 1 {
			return nil
		}
		var row struct {
			Status       string
			PausedReason *string
		}
		read := tx.Raw(`SELECT status, paused_reason FROM operation.batch WHERE id = ?::uuid AND NOT is_delete`,
			batchID.String()).Scan(&row)
		if read.Error != nil {
			return fmt.Errorf("read paused workflow batch: %w", read.Error)
		}
		if read.RowsAffected == 1 && row.Status == string(domain.BatchStatusRunning) &&
			row.PausedReason != nil && *row.PausedReason == reason {
			return nil
		}
		return application.ErrInvalidWorkflowBatch
	})
}

// ResumeWorkflowBatch clears the admission guard after a resume signal.
func (s *Store) ResumeWorkflowBatch(ctx context.Context, batchID uuid.UUID) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if batchID == uuid.Nil {
		return application.ErrInvalidWorkflowBatch
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updated := tx.Exec(`
			UPDATE operation.batch SET paused_reason = NULL, update_time = now()
			WHERE id = ?::uuid AND status = 'running' AND paused_reason IS NOT NULL
			  AND cancel_requested_at IS NULL AND NOT is_delete
		`, batchID.String())
		if updated.Error != nil {
			return fmt.Errorf("resume workflow batch: %w", updated.Error)
		}
		if updated.RowsAffected == 1 {
			return nil
		}
		var row struct {
			Status            string
			PausedReason      *string
			CancelRequestedAt *time.Time
		}
		read := tx.Raw(`SELECT status, paused_reason, cancel_requested_at FROM operation.batch WHERE id = ?::uuid AND NOT is_delete`,
			batchID.String()).Scan(&row)
		if read.Error != nil {
			return fmt.Errorf("read resumed workflow batch: %w", read.Error)
		}
		if read.RowsAffected == 1 && row.Status == string(domain.BatchStatusRunning) &&
			row.PausedReason == nil && row.CancelRequestedAt == nil {
			return nil
		}
		return application.ErrInvalidWorkflowBatch
	})
}

// MarkWorkflowBatchCancelRequested durably closes child admission before the
// parent settles unstarted children or signals work already in flight.
func (s *Store) MarkWorkflowBatchCancelRequested(ctx context.Context, batchID uuid.UUID) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if batchID == uuid.Nil {
		return application.ErrInvalidWorkflowBatch
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updated := tx.Exec(`
			UPDATE operation.batch SET cancel_requested_at = now(), update_time = now()
			WHERE id = ?::uuid AND status = 'running' AND cancel_requested_at IS NULL AND NOT is_delete
		`, batchID.String())
		if updated.Error != nil {
			return fmt.Errorf("mark batch cancellation: %w", updated.Error)
		}
		if updated.RowsAffected == 1 {
			return nil
		}
		var row struct {
			Status            string
			CancelRequestedAt *time.Time
		}
		read := tx.Raw(`SELECT status, cancel_requested_at FROM operation.batch WHERE id = ?::uuid AND NOT is_delete`,
			batchID.String()).Scan(&row)
		if read.Error != nil {
			return fmt.Errorf("read batch cancellation: %w", read.Error)
		}
		if read.RowsAffected == 1 && row.CancelRequestedAt != nil &&
			(row.Status == string(domain.BatchStatusRunning) || row.Status == string(domain.BatchStatusCancelled)) {
			return nil
		}
		return application.ErrInvalidWorkflowBatch
	})
}

// FinishWorkflowBatch writes the final selected-item counts and one summary
// Outbox event atomically. A replay of an already finished batch is read only.
func (s *Store) FinishWorkflowBatch(ctx context.Context, batchID uuid.UUID) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if batchID == uuid.Nil {
		return application.ErrInvalidWorkflowBatch
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch struct {
			ProjectID         uuid.UUID
			OrgID             uuid.UUID
			Kind              string
			Status            string
			TotalCount        int32
			CancelRequestedAt *time.Time
		}
		read := tx.Raw(`
			SELECT b.project_id, p.org_id, b.kind, b.status, b.total_count, b.cancel_requested_at
			FROM operation.batch AS b
			JOIN workspace.project AS p ON p.id = b.project_id
			WHERE b.id = ?::uuid AND NOT b.is_delete AND NOT p.is_delete
			FOR UPDATE OF b
		`, batchID.String()).Scan(&batch)
		if read.Error != nil {
			return fmt.Errorf("lock batch finish: %w", read.Error)
		}
		if read.RowsAffected != 1 {
			return ErrNotFound
		}
		if batch.Status == string(domain.BatchStatusFinished) || batch.Status == string(domain.BatchStatusCancelled) {
			return nil
		}
		if batch.Status != string(domain.BatchStatusRunning) || batch.TotalCount < 1 || batch.TotalCount > 300 {
			return application.ErrInvalidWorkflowBatch
		}
		var children []struct{ Status string }
		if err := tx.Raw(`
			SELECT status FROM operation.operation
			WHERE batch_id = ?::uuid AND project_id = ?::uuid
			  AND NOT is_delete AND status <> 'expired'
		`, batchID.String(), batch.ProjectID.String()).Scan(&children).Error; err != nil {
			return fmt.Errorf("read batch finish children: %w", err)
		}
		if len(children) != int(batch.TotalCount) {
			return application.ErrInvalidWorkflowBatch
		}
		var succeeded, failed, cancelled int32
		for _, child := range children {
			switch domain.Status(child.Status) {
			case domain.StatusCompleted:
				succeeded++
			case domain.StatusFailed:
				failed++
			case domain.StatusCancelled:
				cancelled++
			default:
				return application.ErrWorkflowBatchNotReady
			}
		}
		var hasActiveLaunch bool
		if err := tx.Raw(`
			SELECT EXISTS (
			  SELECT 1 FROM operation.batch_launch
			  WHERE batch_id = ?::uuid AND state = 'active'
			)
		`, batchID.String()).Scan(&hasActiveLaunch).Error; err != nil {
			return fmt.Errorf("check active batch launch slots: %w", err)
		}
		if hasActiveLaunch {
			return application.ErrWorkflowBatchNotReady
		}
		terminalStatus := domain.BatchStatusFinished
		if batch.CancelRequestedAt != nil {
			terminalStatus = domain.BatchStatusCancelled
		}
		updated := tx.Exec(`
			UPDATE operation.batch
			SET status = ?, succeeded_count = ?, failed_count = ?,
			    unknown_count = 0, paused_reason = NULL, update_time = now()
			WHERE id = ?::uuid AND status = 'running' AND NOT is_delete
		`, string(terminalStatus), succeeded, failed, batchID.String())
		if updated.Error != nil {
			return fmt.Errorf("finish workflow batch: %w", updated.Error)
		}
		if updated.RowsAffected != 1 {
			return application.ErrInvalidWorkflowBatch
		}
		const topic = "lanverse.batch.finished.v1"
		eventID := uuid.New()
		event := batchFinishedEvent{
			EventID: eventID, EventType: topic, OccurredAt: time.Now().UTC(),
			OrgID: batch.OrgID, ProjectID: batch.ProjectID,
		}
		event.Actor.Kind = "system"
		event.Aggregate.Type, event.Aggregate.ID = "batch", batchID
		event.Data.BatchID, event.Data.Kind, event.Data.TotalCount = batchID, batch.Kind, batch.TotalCount
		event.Data.Status = string(terminalStatus)
		event.Data.SucceededCount, event.Data.FailedCount, event.Data.CancelledCount = succeeded, failed, cancelled
		payload, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("encode batch finished event: %w", err)
		}
		outbox := tx.Exec(`
			INSERT INTO infra.outbox (id, topic, partition_key, payload)
			VALUES (?::uuid, ?, ?, ?::jsonb)
		`, eventID.String(), topic, batch.ProjectID.String(), string(payload))
		if outbox.Error != nil {
			return fmt.Errorf("insert batch finished event: %w", outbox.Error)
		}
		if outbox.RowsAffected != 1 {
			return application.ErrInvalidWorkflowBatch
		}
		return nil
	})
}
