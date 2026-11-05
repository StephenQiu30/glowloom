package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// VerifyTransferDelivery proves the permanent command and exact original outbox
// data before any dispatch. It rechecks the creator's current owning authority.
func (s *TransferStore) VerifyTransferDelivery(ctx context.Context, d application.TransferDelivery) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	if d.EventID == uuid.Nil || d.JobID == uuid.Nil || d.RequestID == uuid.Nil || d.ActorID == uuid.Nil || d.OrgID == uuid.Nil || d.Attempt < 1 || d.ExecutionID != "" || d.Reconcile != (d.Action == application.TransferActionReconcile) || d.Action != application.TransferActionStart && d.Action != application.TransferActionCancel && d.Action != application.TransferActionReconcile {
		return false, domain.ErrInvalidLibrary
	}
	valid := false
	err := taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		var proof outboxRecord
		read := tx.Table("media.transfer_command AS c").Select("o.payload").
			Joins("JOIN media.transfer_job AS j ON j.id = c.job_id").
			Joins("JOIN infra.outbox AS o ON o.id = c.event_id").
			Where("c.actor_id = ? AND c.org_id = ? AND c.idem_key = ? AND c.event_id = ? AND c.event_action = ? AND c.event_attempt = ?", d.ActorID, d.OrgID, d.RequestID, d.EventID, d.Action, d.Attempt).
			Where("j.id = ? AND j.org_id = c.org_id AND j.actor_id = c.actor_id", d.JobID).
			Where("o.topic = ? AND o.partition_key = ?", TransferTopic, d.JobID.String()).Take(&proof)
		if errors.Is(read.Error, gorm.ErrRecordNotFound) {
			return domain.ErrInvalidLibrary
		}
		if read.Error != nil {
			return read.Error
		}
		var envelope struct {
			Data application.TransferDelivery `json:"data"`
		}
		if json.Unmarshal(proof.Payload, &envelope) != nil || !reflect.DeepEqual(envelope.Data, d) {
			return domain.ErrInvalidLibrary
		}
		actor, err := s.workerActor(tx, d.JobID)
		if err != nil {
			return err
		}
		if actor.ID != d.ActorID || actor.OrgID != d.OrgID {
			return domain.ErrInvalidLibrary
		}
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		row, err := s.readJob(tx, actor, d.JobID, false, true)
		if err != nil {
			return err
		}
		source, target := row.scopes()
		if _, err := s.access(ctx, tx, actor, source, target, true); err != nil {
			return err
		}
		valid = row.Attempt == d.Attempt && row.Status != domain.TransferStatusSucceeded && row.Status != domain.TransferStatusCancelled && row.Status != domain.TransferStatusFailed && row.Status != domain.TransferStatusPartialFailed
		return nil
	})
	return valid, err
}

// InterruptTransfer preserves a lost orchestration fence without inventing
// physical cessation. Only EndTransferPhysical can prove the synchronous owner
// has joined all calls; an already ended owner remains ended after late delivery.
func (s *TransferStore) InterruptTransfer(ctx context.Context, work application.TransferWorkID) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	execution, err := uuid.Parse(work.ExecutionID)
	if err != nil || execution == uuid.Nil || work.JobID == uuid.Nil || work.Attempt < 1 {
		return domain.ErrTransferConflict
	}
	return taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		var row transferJobRow
		read := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", work.JobID).Take(&row)
		if errors.Is(read.Error, gorm.ErrRecordNotFound) {
			return application.ErrNotFound
		}
		if read.Error != nil {
			return read.Error
		}
		if row.Attempt != work.Attempt {
			return domain.ErrTransferConflict
		}
		if row.ExecutionID == nil {
			return nil
		}
		if *row.ExecutionID != execution || row.WorkerFence == nil {
			return domain.ErrTransferConflict
		}
		if row.Status == domain.TransferStatusSucceeded || row.Status == domain.TransferStatusCancelled || row.Status == domain.TransferStatusFailed || row.Status == domain.TransferStatusPartialFailed {
			return nil
		}
		uncertain := !row.ProcessEnded
		if row.Status == domain.TransferStatusNeedsReconciliation && row.NeedsReconciliation && row.ExecutionUnconfirmed == uncertain {
			return nil
		}
		return updateOne(tx, "media.transfer_job", map[string]any{"id": row.ID, "attempt": work.Attempt, "execution_id": execution, "worker_fence": *row.WorkerFence, "revision": row.Revision}, map[string]any{"status": domain.TransferStatusNeedsReconciliation, "stage": domain.TransferStageCleanup, "needs_reconciliation": true, "execution_unconfirmed": uncertain, "revision": row.Revision + 1, "updated_at": s.clock().UTC()}, application.ErrLibraryConflict)
	})
}
