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

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
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
	err := taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if _, err := s.access(ctx, tx, actor, scope, false); err != nil {
			return err
		}
		library, _ := scope.Identity(actor.OrgID, actor.ID)
		var rows []purgeJobRow
		if err := tx.Where("actor_id = ? AND org_id = ? AND library_id = ?", actor.ID, actor.OrgID, library).
			Order("created_at DESC").Order("id").Limit(size).Offset((page - 1) * size).Find(&rows).Error; err != nil {
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
	if d.EventID == uuid.Nil || d.JobID == uuid.Nil || d.RequestID == uuid.Nil || d.ActorID == uuid.Nil || d.OrgID == uuid.Nil || d.Attempt < 1 || d.ExecutionID != "" || d.Action != application.PurgeActionCreate && d.Action != application.PurgeActionCancel && d.Action != application.PurgeActionReconcile {
		return false, domain.ErrInvalidLibrary
	}
	valid := false
	err := taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		var proof struct{ Payload jsonDocument }
		read := tx.Table("media.purge_command AS c").Select("o.payload").
			Joins("JOIN media.purge_job AS j ON j.id = c.job_id").
			Joins("JOIN infra.outbox AS o ON o.id = c.event_id").
			Where("c.actor_id = ? AND c.org_id = ? AND c.idem_key = ? AND c.event_id = ? AND c.action = ?", d.ActorID, d.OrgID, d.RequestID, d.EventID, d.Action).
			Where("j.id = ? AND j.org_id = c.org_id AND j.actor_id = c.actor_id", d.JobID).
			Where("o.topic = ? AND o.partition_key = ?", PurgeTopic, d.JobID.String()).Take(&proof)
		if errors.Is(read.Error, gorm.ErrRecordNotFound) {
			return domain.ErrInvalidLibrary
		}
		if read.Error != nil {
			return read.Error
		}
		var envelope struct {
			Data application.PurgeDelivery `json:"data"`
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
		row, err := s.readJob(tx, actor, d.JobID, false)
		if err != nil {
			return err
		}
		if _, err := s.access(ctx, tx, actor, row.scope(), true); err != nil {
			return err
		}
		valid = row.Attempt == d.Attempt && row.Status != domain.PurgeStatusSucceeded && row.Status != domain.PurgeStatusPartialFailed && row.Status != domain.PurgeStatusFailed && row.Status != domain.PurgeStatusCancelled
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
	return taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		var row purgeJobRow
		read := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", work.JobID).Take(&row)
		if read.Error != nil {
			if errors.Is(read.Error, gorm.ErrRecordNotFound) {
				return application.ErrNotFound
			}
			return read.Error
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
		if row.Status == domain.PurgeStatusSucceeded || row.Status == domain.PurgeStatusPartialFailed || row.Status == domain.PurgeStatusFailed || row.Status == domain.PurgeStatusCancelled {
			return nil
		}
		uncertain := !row.ProcessEnded
		if row.Status == domain.PurgeStatusNeedsReconciliation && row.NeedsReconciliation && row.ExecutionUnconfirmed == uncertain {
			return nil
		}
		return updateOne(tx, "media.purge_job", map[string]any{"id": row.ID, "attempt": work.Attempt, "execution_id": execution, "worker_fence": *row.WorkerFence, "revision": row.Revision}, map[string]any{"status": domain.PurgeStatusNeedsReconciliation, "stage": domain.PurgeStageRemoving, "needs_reconciliation": true, "execution_unconfirmed": uncertain, "revision": row.Revision + 1, "updated_at": s.clock().UTC()}, application.ErrLibraryConflict)
	})
}
