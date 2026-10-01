package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

// AcquireBatchLaunch reserves one durable project/provider slot before the
// parent starts a child. A false result means capacity or state is not ready.
// The operation ID is the idempotency key when an Activity response is lost.
func (s *Store) AcquireBatchLaunch(ctx context.Context, batchID, operationID uuid.UUID) (bool, error) {
	if s == nil || s.db == nil {
		return false, ErrUnavailable
	}
	if batchID == uuid.Nil || operationID == uuid.Nil {
		return false, application.ErrInvalidWorkflowBatch
	}
	var acquired bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var item struct {
			ProjectID             uuid.UUID
			BatchID               *uuid.UUID
			ModelProfileVersionID *uuid.UUID
			ReusedFromID          *uuid.UUID
		}
		read := tx.Raw(`
			SELECT project_id, batch_id, model_profile_version_id, reused_from_id
			FROM operation.operation
			WHERE id = ?::uuid AND NOT is_delete
		`, operationID.String()).Scan(&item)
		if read.Error != nil {
			return fmt.Errorf("read batch launch member: %w", read.Error)
		}
		if read.RowsAffected != 1 || item.BatchID == nil || *item.BatchID != batchID {
			return application.ErrInvalidWorkflowBatch
		}
		var providerID *uuid.UUID
		providerLimit := 0
		if item.ReusedFromID == nil {
			if item.ModelProfileVersionID == nil {
				return application.ErrWorkflowModelUnavailable
			}
			var provider struct {
				ID               uuid.UUID
				ConcurrencyLimit int
			}
			read = tx.Raw(`
				SELECT p.id, p.concurrency_limit FROM catalog.model_profile_version AS v
				JOIN catalog.model_profile AS m ON m.id = v.model_profile_id
				JOIN catalog.provider AS p ON p.id = m.provider_id
				WHERE v.id = ?::uuid AND NOT v.is_delete AND NOT m.is_delete
				  AND NOT p.is_delete AND p.status = 'active'
			`, item.ModelProfileVersionID.String()).Scan(&provider)
			if read.Error != nil {
				return fmt.Errorf("read batch provider capacity: %w", read.Error)
			}
			if read.RowsAffected != 1 || provider.ID == uuid.Nil || provider.ConcurrencyLimit < 1 {
				return application.ErrWorkflowModelUnavailable
			}
			providerID = &provider.ID
			if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?::text, 0))`,
				provider.ID.String()).Error; err != nil {
				return fmt.Errorf("lock batch provider admission: %w", err)
			}
			read = tx.Raw(`
				SELECT concurrency_limit FROM catalog.provider
				WHERE id = ?::uuid AND NOT is_delete AND status = 'active'
			`, provider.ID.String()).Scan(&providerLimit)
			if read.Error != nil {
				return fmt.Errorf("recheck batch provider capacity: %w", read.Error)
			}
			if read.RowsAffected != 1 || providerLimit < 1 {
				return application.ErrWorkflowModelUnavailable
			}
		}
		var project struct{ ID uuid.UUID }
		read = tx.Raw(`
			SELECT id FROM workspace.project
			WHERE id = ?::uuid AND NOT is_delete FOR UPDATE
		`, item.ProjectID.String()).Scan(&project)
		if read.Error != nil {
			return fmt.Errorf("lock batch project capacity: %w", read.Error)
		}
		if read.RowsAffected != 1 {
			return application.ErrInvalidWorkflowBatch
		}
		var batch struct {
			Status            string
			PausedReason      *string
			CancelRequestedAt *time.Time
			Scope             string
		}
		read = tx.Raw(`
			SELECT status, paused_reason, cancel_requested_at,scope::text AS scope FROM operation.batch
			WHERE id = ?::uuid AND project_id = ?::uuid AND NOT is_delete FOR UPDATE
		`, batchID.String(), item.ProjectID.String()).Scan(&batch)
		if read.Error != nil {
			return fmt.Errorf("lock batch launch state: %w", read.Error)
		}
		if read.RowsAffected != 1 || batch.Status != "running" || batch.PausedReason != nil ||
			batch.CancelRequestedAt != nil {
			return nil
		}
		var operation struct {
			Status        string
			ReservationID *uuid.UUID
			WorkflowID    *string
		}
		read = tx.Raw(`
			SELECT status, reservation_id, workflow_id FROM operation.operation
			WHERE id = ?::uuid AND batch_id = ?::uuid AND NOT is_delete FOR UPDATE
		`, operationID.String(), batchID.String()).Scan(&operation)
		if read.Error != nil {
			return fmt.Errorf("lock batch launch operation: %w", read.Error)
		}
		if read.RowsAffected != 1 || operation.Status != "confirmed" {
			return nil
		}
		if operation.ReservationID == nil || *operation.ReservationID == uuid.Nil ||
			operation.WorkflowID == nil || *operation.WorkflowID != "operation/"+operationID.String() {
			return application.ErrInvalidWorkflowBatch
		}
		var existing struct{ State string }
		read = tx.Raw(`
			SELECT state FROM operation.batch_launch WHERE operation_id = ?::uuid FOR UPDATE
		`, operationID.String()).Scan(&existing)
		if read.Error != nil {
			return fmt.Errorf("read existing batch launch: %w", read.Error)
		}
		if read.RowsAffected == 1 {
			if existing.State != "active" {
				return application.ErrInvalidWorkflowBatch
			}
			acquired = true
			return nil
		}
		var frozenScope struct {
			NodeID      *uuid.UUID `json:"node_id"`
			Concurrency int        `json:"concurrency"`
		}
		if err := json.Unmarshal([]byte(batch.Scope), &frozenScope); err != nil {
			return application.ErrInvalidWorkflowBatch
		}
		if frozenScope.NodeID != nil {
			if *frozenScope.NodeID == uuid.Nil || frozenScope.Concurrency < 1 || frozenScope.Concurrency > 32 {
				return application.ErrInvalidWorkflowBatch
			}
			var activeBatch int64
			if err := tx.Raw(`SELECT count(*) FROM operation.batch_launch WHERE batch_id=?::uuid AND state='active'`, batchID).Scan(&activeBatch).Error; err != nil {
				return fmt.Errorf("count saved canvas batch capacity: %w", err)
			}
			if activeBatch >= int64(frozenScope.Concurrency) {
				return nil
			}
		}
		var activeProject int64
		if err := tx.Raw(`
			SELECT count(*) FROM operation.batch_launch
			WHERE project_id = ?::uuid AND state = 'active'
		`, item.ProjectID.String()).Scan(&activeProject).Error; err != nil {
			return fmt.Errorf("count active project batch launches: %w", err)
		}
		if activeProject >= 20 {
			return nil
		}
		if providerID != nil {
			var activeProvider int64
			if err := tx.Raw(`
				SELECT count(*) FROM operation.batch_launch
				WHERE provider_id = ?::uuid AND state = 'active'
			`, providerID.String()).Scan(&activeProvider).Error; err != nil {
				return fmt.Errorf("count active provider batch launches: %w", err)
			}
			if activeProvider >= int64(providerLimit) {
				return nil
			}
		}
		insert := tx.Exec(`
			INSERT INTO operation.batch_launch
			  (operation_id, batch_id, project_id, provider_id, state)
			VALUES (?::uuid, ?::uuid, ?::uuid, ?::uuid, 'active')
		`, operationID.String(), batchID.String(), item.ProjectID.String(), providerID)
		if insert.Error != nil {
			return fmt.Errorf("reserve batch launch slot: %w", insert.Error)
		}
		if insert.RowsAffected != 1 {
			return application.ErrInvalidWorkflowBatch
		}
		acquired = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("acquire batch launch: %w", err)
	}
	return acquired, nil
}

// ReleaseBatchLaunchInTransaction frees an active slot in the same transaction
// that records a child operation's terminal state and billing settlement.
func (s *Store) ReleaseBatchLaunchInTransaction(ctx context.Context, tx *gorm.DB, operationID uuid.UUID) error {
	if s == nil || s.db == nil || tx == nil || operationID == uuid.Nil {
		return application.ErrInvalidWorkflowBatch
	}
	result := tx.WithContext(ctx).Exec(`
		UPDATE operation.batch_launch AS l
		SET state = 'done', released_at = now()
		WHERE l.operation_id = ?::uuid AND l.state = 'active'
		  AND EXISTS (
		    SELECT 1 FROM operation.operation AS o
		    WHERE o.id = l.operation_id
		      AND o.status IN ('completed', 'failed', 'cancelled')
		      AND o.settled_micros IS NOT NULL
		  )
	`, operationID.String())
	if result.Error != nil {
		return fmt.Errorf("release batch launch slot: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		var stillActive bool
		if err := tx.WithContext(ctx).Raw(`
			SELECT EXISTS (
			  SELECT 1 FROM operation.batch_launch
			  WHERE operation_id = ?::uuid AND state = 'active'
			)
		`, operationID.String()).Scan(&stillActive).Error; err != nil {
			return fmt.Errorf("check unreleased batch launch slot: %w", err)
		}
		if stillActive {
			return application.ErrInvalidWorkflowBatch
		}
	}
	return nil
}
