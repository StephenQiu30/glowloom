package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// VerifyDelivery requires the exact immutable command outbox and its current org-bound job.
func (s *ProjectCopyStore) VerifyDelivery(ctx context.Context, d application.ProjectCopyDelivery) (bool, error) {
	if s == nil || s.db == nil || d.EventID == uuid.Nil || d.OrgID == uuid.Nil || d.JobID == uuid.Nil {
		return false, domain.ErrInvalidProjectCopy
	}
	var valid bool
	err := s.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM infra.outbox o JOIN workspace.project_copy_job j ON j.id=? AND j.org_id=? WHERE o.id=? AND o.topic='lanverse.workspace.project_copy_requested.v1' AND o.partition_key=? AND o.payload->>'org_id'=? AND o.payload->'data'->>'copy_job_id'=? AND o.payload->'data'->>'source_project_id'=? AND o.payload->'data'->>'target_project_id'=? AND (o.payload->'data'->>'revision')::bigint=? AND o.payload->'data'->>'action'=? AND o.payload->'data'->>'status'=? AND o.payload->'data'->>'stage'=? AND o.payload->'actor'->>'id'=? AND o.payload->'actor'->>'kind'='user' AND o.payload->>'occurred_at'=? AND j.source_project_id=? AND j.target_project_id=? AND j.status NOT IN ('succeeded','cancelled'))`, d.JobID, d.OrgID, d.EventID, d.JobID.String(), d.OrgID.String(), d.JobID.String(), d.SourceProjectID.String(), d.TargetProjectID.String(), d.Revision, d.Action, d.Status, d.Stage, d.ActorID.String(), d.OccurredAt.UTC().Format(time.RFC3339Nano), d.SourceProjectID, d.TargetProjectID).Scan(&valid).Error
	return valid, err
}

// Interrupt records timeout uncertainty while retaining the unacknowledged original worker.
func (s *ProjectCopyStore) Interrupt(ctx context.Context, id application.ProjectCopyWorkID) error {
	actor, err := s.WorkerActor(ctx, id.OrgID, id.JobID)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		before, err := readCopyJob(tx, actor, id.JobID, true)
		if err != nil {
			return err
		}
		if before.WorkerID != id.WorkerID || before.WorkerID == uuid.Nil || before.ExecutionUnconfirmed || before.Status == "succeeded" || before.Status == "cancelled" {
			return nil
		}
		after := before
		if err := after.Interrupt(id.WorkerID); err != nil {
			return err
		}
		if err := saveCopyJob(tx, before, after); err != nil {
			return err
		}
		return copyChanged(tx, after, "failed", time.Now())
	})
}
