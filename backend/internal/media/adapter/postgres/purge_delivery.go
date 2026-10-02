package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// ListPurges keeps history creator-scoped and rechecks project read authority.
func (s *PurgeStore) ListPurges(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope, page, size int) ([]domain.PurgeJob, error) {
	if s == nil || s.db == nil {
		return nil, application.ErrUnavailable
	}
	if scope.Validate() != nil || page < 1 || page > 10000 || size < 1 || size > 100 {
		return nil, domain.ErrInvalidLibrary
	}
	result := make([]domain.PurgeJob, 0, size)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if _, err := s.access(ctx, tx, actor, scope, false); err != nil {
			return err
		}
		library, _ := scope.Identity(actor.OrgID, actor.ID)
		var rows []purgeJobRow
		if err := tx.Raw(`SELECT * FROM media.purge_job WHERE actor_id=? AND org_id=? AND library_id=? ORDER BY created_at DESC,id LIMIT ? OFFSET ?`, actor.ID, actor.OrgID, library, size, (page-1)*size).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			job, err := purgeView(tx, row)
			if err != nil {
				return err
			}
			result = append(result, job)
		}
		return nil
	})
	return result, err
}

// VerifyPurgeDelivery requires the permanent command, exact committed data and
// current creator authority before acknowledging an external dispatch.
func (s *PurgeStore) VerifyPurgeDelivery(ctx context.Context, d application.PurgeDelivery) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	if d.EventID == uuid.Nil || d.JobID == uuid.Nil || d.RequestID == uuid.Nil || d.ActorID == uuid.Nil || d.OrgID == uuid.Nil || d.Attempt < 1 || d.ExecutionID != "" || d.Action != "create" && d.Action != "cancel" && d.Action != "reconcile" {
		return false, domain.ErrInvalidLibrary
	}
	data, err := json.Marshal(d)
	if err != nil {
		return false, err
	}
	valid := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var present bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.purge_command c JOIN media.purge_job j ON j.id=c.job_id JOIN infra.outbox o ON o.id=c.event_id WHERE c.actor_id=? AND c.org_id=? AND c.idem_key=? AND c.event_id=? AND c.action=? AND j.id=? AND j.org_id=c.org_id AND j.actor_id=c.actor_id AND o.topic=? AND o.partition_key=? AND o.payload->'data'=?::jsonb)`, d.ActorID, d.OrgID, d.RequestID, d.EventID, d.Action, d.JobID, PurgeTopic, d.JobID.String(), string(data)).Scan(&present).Error; err != nil {
			return err
		}
		if !present {
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
		if _, err := s.access(ctx, tx, actor, row.scope(), false); err != nil {
			return err
		}
		valid = row.Attempt == d.Attempt && row.Status != "succeeded" && row.Status != "partial_failed" && row.Status != "failed" && row.Status != "cancelled"
		return nil
	})
	return valid, err
}

// InterruptPurge never manufactures physical cessation or releases reservations.
func (s *PurgeStore) InterruptPurge(ctx context.Context, work application.PurgeWorkID) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	execution, err := uuid.Parse(work.ExecutionID)
	if err != nil || execution == uuid.Nil || work.JobID == uuid.Nil || work.Attempt < 1 {
		return domain.ErrPurgeConflict
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row purgeJobRow
		read := tx.Raw(`SELECT * FROM media.purge_job WHERE id=? FOR UPDATE`, work.JobID).Scan(&row)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 {
			return application.ErrNotFound
		}
		if row.Attempt != work.Attempt {
			return domain.ErrPurgeConflict
		}
		if row.ExecutionID == nil {
			return nil
		}
		if *row.ExecutionID != execution || row.WorkerFence == nil {
			return domain.ErrPurgeConflict
		}
		if row.Status == "succeeded" || row.Status == "partial_failed" || row.Status == "failed" || row.Status == "cancelled" {
			return nil
		}
		uncertain := !row.ProcessEnded
		if row.Status == "needs_reconciliation" && row.NeedsReconciliation && row.ExecutionUnconfirmed == uncertain {
			return nil
		}
		return libraryChanged(tx, `UPDATE media.purge_job SET status='needs_reconciliation',stage='removing',needs_reconciliation=true,execution_unconfirmed=?,revision=revision+1,updated_at=? WHERE id=? AND attempt=? AND execution_id=? AND worker_fence=? AND revision=?`, uncertain, s.clock().UTC(), row.ID, work.Attempt, execution, *row.WorkerFence, row.Revision)
	})
}
