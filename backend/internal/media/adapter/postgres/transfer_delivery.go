package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// VerifyTransferDelivery proves the permanent command and exact original outbox
// data before any dispatch. It rechecks the creator's current owning authority.
func (s *TransferStore) VerifyTransferDelivery(ctx context.Context, d application.TransferDelivery) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	if d.EventID == uuid.Nil || d.JobID == uuid.Nil || d.RequestID == uuid.Nil || d.ActorID == uuid.Nil || d.OrgID == uuid.Nil || d.Attempt < 1 || d.ExecutionID != "" || d.Reconcile != (d.Action == "reconcile") || d.Action != "start" && d.Action != "cancel" && d.Action != "reconcile" {
		return false, domain.ErrInvalidLibrary
	}
	body, err := json.Marshal(d)
	if err != nil {
		return false, err
	}
	valid := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var exists bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.transfer_command c JOIN media.transfer_job j ON j.id=c.job_id JOIN infra.outbox o ON o.id=c.event_id WHERE c.actor_id=? AND c.org_id=? AND c.idem_key=? AND c.event_id=? AND c.event_action=? AND c.event_attempt=? AND j.id=? AND j.org_id=c.org_id AND j.actor_id=c.actor_id AND o.topic=? AND o.partition_key=? AND o.payload->'data'=?::jsonb)`, d.ActorID, d.OrgID, d.RequestID, d.EventID, d.Action, d.Attempt, d.JobID, TransferTopic, d.JobID.String(), string(body)).Scan(&exists).Error; err != nil {
			return err
		}
		if !exists {
			return domain.ErrInvalidLibrary
		}
		actor, err := s.workerActor(tx, d.JobID)
		if err != nil {
			return err
		}
		if actor.ID != d.ActorID || actor.OrgID != d.OrgID {
			return domain.ErrInvalidLibrary
		}
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		row, err := s.readJob(tx, actor, d.JobID, false)
		if err != nil {
			return err
		}
		source, target := row.scopes()
		if _, err := s.access(ctx, tx, actor, source, target, false); err != nil {
			return err
		}
		valid = row.Attempt == d.Attempt && row.Status != "succeeded" && row.Status != "cancelled" && row.Status != "failed" && row.Status != "partial_failed"
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
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row transferJobRow
		read := tx.Raw(`SELECT * FROM media.transfer_job WHERE id=? FOR UPDATE`, work.JobID).Scan(&row)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 {
			return application.ErrNotFound
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
		if row.Status == "succeeded" || row.Status == "cancelled" || row.Status == "failed" || row.Status == "partial_failed" {
			return nil
		}
		uncertain := !row.ProcessEnded
		if row.Status == "needs_reconciliation" && row.NeedsReconciliation && row.ExecutionUnconfirmed == uncertain {
			return nil
		}
		return libraryChanged(tx, `UPDATE media.transfer_job SET status='needs_reconciliation',stage='cleanup',needs_reconciliation=true,execution_unconfirmed=?,revision=revision+1,updated_at=? WHERE id=? AND attempt=? AND execution_id=? AND worker_fence=? AND revision=?`, uncertain, s.clock().UTC(), row.ID, work.Attempt, execution, *row.WorkerFence, row.Revision)
	})
}
