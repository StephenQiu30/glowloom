package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

const copyJobColumns = "request_id,id,org_id,actor_id,source_project_id,target_project_id,source_revision,revision,attempt,target_name,status,stage,failure_code,worker_id,started_at,manifest,media_receipt,canvas_receipt,retryable,needs_reconciliation,cancellation_requested,reconciliation_requested,execution_unconfirmed"

type copyJobRow struct {
	CreateTime                                            time.Time
	RequestID                                             string
	ID, OrgID, ActorID, SourceProjectID, TargetProjectID  uuid.UUID
	SourceRevision, Revision, Attempt                     int64
	TargetName, Status, Stage, FailureCode                string
	WorkerID                                              *uuid.UUID
	StartedAt                                             *time.Time
	Manifest, MediaReceipt, CanvasReceipt                 []byte
	Retryable, NeedsReconciliation, CancellationRequested bool
	ReconciliationRequested, ExecutionUnconfirmed         bool
}

func (r copyJobRow) job() (domain.ProjectCopyJob, error) {
	j := domain.ProjectCopyJob{RequestID: r.RequestID, ID: r.ID, OrgID: r.OrgID, ActorID: r.ActorID, SourceProjectID: r.SourceProjectID, TargetProjectID: r.TargetProjectID, SourceRevision: r.SourceRevision, Revision: r.Revision, Attempt: r.Attempt, TargetName: r.TargetName, Status: r.Status, Stage: r.Stage, FailureCode: r.FailureCode, StartedAt: r.StartedAt, Retryable: r.Retryable, NeedsReconciliation: r.NeedsReconciliation, CancellationRequested: r.CancellationRequested, ReconciliationRequested: r.ReconciliationRequested, ExecutionUnconfirmed: r.ExecutionUnconfirmed}
	if r.WorkerID != nil {
		j.WorkerID = *r.WorkerID
	}
	if copyJSON(r.Manifest, &j.Manifest) != nil {
		return domain.ProjectCopyJob{}, domain.ErrInvalidProjectCopy
	}
	if len(r.MediaReceipt) > 0 && copyJSON(r.MediaReceipt, &j.MediaReceipt) != nil {
		return domain.ProjectCopyJob{}, domain.ErrInvalidProjectCopy
	}
	if len(r.CanvasReceipt) > 0 && copyJSON(r.CanvasReceipt, &j.CanvasReceipt) != nil {
		return domain.ProjectCopyJob{}, domain.ErrInvalidProjectCopy
	}
	return j, j.Validate()
}

func readCopyJob(tx *gorm.DB, actor identityapp.Principal, id uuid.UUID, write bool) (domain.ProjectCopyJob, error) {
	var row copyJobRow
	read := tx.Raw(`SELECT `+copyJobColumns+` FROM workspace.project_copy_job WHERE id=? AND org_id=?`, id, actor.OrgID).Scan(&row)
	if read.Error != nil {
		return domain.ProjectCopyJob{}, read.Error
	}
	if read.RowsAffected != 1 {
		return domain.ProjectCopyJob{}, application.ErrProjectNotFound
	}
	if write {
		// Lock both projects before the job. All checkpoints use this order, so
		// target publication never upgrades a shared lock behind another worker.
		var projects []struct{ ID uuid.UUID }
		if err := tx.Raw(`SELECT id FROM workspace.project WHERE org_id=? AND id IN (?,?) ORDER BY id FOR UPDATE`, actor.OrgID, row.SourceProjectID, row.TargetProjectID).Scan(&projects).Error; err != nil {
			return domain.ProjectCopyJob{}, err
		}
		if len(projects) != 2 {
			return domain.ProjectCopyJob{}, application.ErrProjectNotFound
		}
		read = tx.Raw(`SELECT `+copyJobColumns+` FROM workspace.project_copy_job WHERE id=? AND org_id=? FOR UPDATE`, id, actor.OrgID).Scan(&row)
		if read.Error != nil {
			return domain.ProjectCopyJob{}, read.Error
		}
	}
	return row.job()
}

// Find reads a current job in the actor's organization without private snapshot data.
func (s *ProjectCopyStore) Find(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (domain.ProjectCopyJob, error) {
	if s == nil || s.db == nil || id == uuid.Nil {
		return domain.ProjectCopyJob{}, application.ErrProjectDependencyUnavailable
	}
	var saved domain.ProjectCopyJob
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		var err error
		saved, err = readCopyJob(tx, actor, id, false)
		return err
	})
	return saved, err
}

func saveCopyJob(tx *gorm.DB, before, after domain.ProjectCopyJob) error {
	if after.Validate() != nil || after.Revision != before.Revision+1 {
		return domain.ErrInvalidProjectCopy
	}
	var media, canvas any
	if after.MediaReceipt != nil {
		body, err := json.Marshal(after.MediaReceipt)
		if err != nil {
			return err
		}
		media = string(body)
	}
	if after.CanvasReceipt != nil {
		body, err := json.Marshal(after.CanvasReceipt)
		if err != nil {
			return err
		}
		canvas = string(body)
	}
	var worker any
	if after.WorkerID != uuid.Nil {
		worker = after.WorkerID
	}
	update := tx.Exec(`UPDATE workspace.project_copy_job SET status=?,stage=?,revision=?,attempt=?,worker_id=?,started_at=?,media_receipt=?::jsonb,canvas_receipt=?::jsonb,failure_code=?,retryable=?,needs_reconciliation=?,cancellation_requested=?,reconciliation_requested=?,execution_unconfirmed=?,update_time=statement_timestamp() WHERE id=? AND org_id=? AND revision=? AND worker_id IS NOT DISTINCT FROM ?::uuid`, after.Status, after.Stage, after.Revision, after.Attempt, worker, after.StartedAt, media, canvas, after.FailureCode, after.Retryable, after.NeedsReconciliation, after.CancellationRequested, after.ReconciliationRequested, after.ExecutionUnconfirmed, after.ID, after.OrgID, before.Revision, nullableCopyWorker(before.WorkerID))
	if update.Error != nil {
		return fmt.Errorf("persist project copy checkpoint: %w", update.Error)
	}
	if update.RowsAffected != 1 {
		return domain.ErrProjectCopyWorkerConflict
	}
	return nil
}

func nullableCopyWorker(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}

func copyChanged(tx *gorm.DB, job domain.ProjectCopyJob, action string, now time.Time) error {
	id := uuid.New()
	topic := "lanverse.workspace.project_copy_changed.v1"
	if action == "requested" || action == "retry" || action == "cancel" || action == "reconcile" {
		topic = "lanverse.workspace.project_copy_requested.v1"
	}
	payload, err := json.Marshal(map[string]any{"event_id": id, "event_type": topic, "occurred_at": now.UTC(), "org_id": job.OrgID, "project_id": job.SourceProjectID, "actor": map[string]any{"kind": "user", "id": job.ActorID}, "aggregate": map[string]any{"type": "project_copy", "id": job.ID, "revision": job.Revision}, "data": map[string]any{"copy_job_id": job.ID, "source_project_id": job.SourceProjectID, "target_project_id": job.TargetProjectID, "status": job.Status, "stage": job.Stage, "revision": job.Revision, "action": action}})
	if err != nil {
		return err
	}
	if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, id, topic, job.ID.String(), string(payload)).Error; err != nil {
		return err
	}
	terminal := map[string]string{"published": "project.copy_completed", "failed": "project.copy_failed", "cancelled": "project.copy_cancelled"}[action]
	if terminal != "" {
		return copyAudit(tx, job, identityapp.Principal{ID: job.ActorID, OrgID: job.OrgID}, terminal, job.RequestID, now)
	}
	return nil
}

// Claim claims one waiting attempt or an explicit same-key reconciliation attempt.
func (s *ProjectCopyStore) Claim(ctx context.Context, actor identityapp.Principal, id, worker uuid.UUID, reconcile bool, now time.Time) (domain.ProjectCopyJob, error) {
	var saved domain.ProjectCopyJob
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		before, err := readCopyJob(tx, actor, id, true)
		if err != nil {
			return err
		}
		saved = before
		if reconcile {
			err = saved.ResumeReconciliation(worker)
		} else {
			err = saved.Start(worker, now)
		}
		if err != nil {
			return err
		}
		if err := saveCopyJob(tx, before, saved); err != nil {
			return err
		}
		return copyChanged(tx, saved, "claimed", now)
	})
	return saved, err
}

// UseAttempt runs an owner checkpoint only under a live worker and exact stage fence.
// The callback must perform database work only; external bytes run after this transaction.
func (s *ProjectCopyStore) UseAttempt(ctx context.Context, actor identityapp.Principal, id, worker uuid.UUID, stage string, use func(application.ProjectCopyOwners, domain.ProjectCopyJob) error) error {
	if s == nil || s.db == nil || use == nil || worker == uuid.Nil {
		return application.ErrProjectDependencyUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		job, err := readCopyJob(tx, actor, id, true)
		if err != nil {
			return err
		}
		if job.WorkerID != worker {
			return domain.ErrProjectCopyWorkerConflict
		}
		if job.Stage != stage || (stage == "cleanup" && job.Status != "cancel_requested") || (stage != "cleanup" && (job.Status != "running" || job.CancellationRequested)) {
			return domain.ErrProjectCopyStateConflict
		}
		owners, err := s.transactionOwners(tx)
		if err != nil {
			return err
		}
		return use(owners, job)
	})
}

// CompleteMedia registers all checked objects and advances the same job atomically.
func (s *ProjectCopyStore) CompleteMedia(ctx context.Context, actor identityapp.Principal, id, worker uuid.UUID) (domain.ProjectCopyJob, error) {
	return s.completeOwner(ctx, actor, id, worker, "media")
}

// CompleteCanvases writes every private graph and advances the same job atomically.
func (s *ProjectCopyStore) CompleteCanvases(ctx context.Context, actor identityapp.Principal, id, worker uuid.UUID) (domain.ProjectCopyJob, error) {
	return s.completeOwner(ctx, actor, id, worker, "canvases")
}

func (s *ProjectCopyStore) completeOwner(ctx context.Context, actor identityapp.Principal, id, worker uuid.UUID, stage string) (domain.ProjectCopyJob, error) {
	var saved domain.ProjectCopyJob
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		before, err := readCopyJob(tx, actor, id, true)
		if err != nil {
			return err
		}
		if before.WorkerID != worker || worker == uuid.Nil {
			return domain.ErrProjectCopyWorkerConflict
		}
		if before.Stage != stage || before.Status != "running" || before.CancellationRequested {
			return domain.ErrProjectCopyStateConflict
		}
		owners, err := s.transactionOwners(tx)
		if err != nil {
			return err
		}
		saved = before
		if stage == "media" {
			receipt, err := owners.Media.Register(ctx, actor, copyMediaBinding(before), copyMediaSnapshot(before))
			if err != nil {
				return err
			}
			if err := saved.AcceptMediaReceipt(worker, domain.ProjectCopyReceipt{ManifestSHA256: receipt.ManifestSHA256, ContentSHA256: receipt.ContentSHA256, PrimaryCount: receipt.Assets, SecondaryCount: receipt.Renditions}); err != nil {
				return err
			}
		} else {
			receipt, err := owners.Canvas.Copy(ctx, actor, copyCanvasBinding(before), copyCanvasSnapshot(before))
			if err != nil {
				return err
			}
			if err := saved.AcceptCanvasReceipt(worker, domain.ProjectCopyReceipt{ManifestSHA256: receipt.ManifestSHA256, ContentSHA256: receipt.ContentSHA256, PrimaryCount: receipt.Documents, SecondaryCount: receipt.ClearedOperationBindings}); err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
		if err := saveCopyJob(tx, before, saved); err != nil {
			return err
		}
		return copyChanged(tx, saved, "checkpoint", time.Now())
	})
	return saved, err
}

// Publish activates the entire target and completes the job in the same transaction.
func (s *ProjectCopyStore) Publish(ctx context.Context, actor identityapp.Principal, id, worker uuid.UUID) (domain.ProjectCopyJob, error) {
	var saved domain.ProjectCopyJob
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		before, err := readCopyJob(tx, actor, id, true)
		if err != nil {
			return err
		}
		saved = before
		if err := saved.Publish(worker); err != nil {
			return err
		}
		if err := verifyCopyWorkspace(tx, before); err != nil {
			return err
		}
		owners, err := s.transactionOwners(tx)
		if err != nil {
			return err
		}
		media, err := owners.Media.Register(ctx, actor, copyMediaBinding(before), copyMediaSnapshot(before))
		if err != nil {
			return err
		}
		canvas, err := owners.Canvas.Copy(ctx, actor, copyCanvasBinding(before), copyCanvasSnapshot(before))
		if err != nil {
			return err
		}
		if before.MediaReceipt == nil || before.CanvasReceipt == nil || media.ContentSHA256 != before.MediaReceipt.ContentSHA256 || canvas.ContentSHA256 != before.CanvasReceipt.ContentSHA256 {
			return domain.ErrInvalidProjectCopy
		}
		if err := owners.Budget.VerifyCopyTargetBudget(ctx, actor, before.TargetProjectID); err != nil {
			return err
		}
		update := tx.Exec(`UPDATE workspace.project SET status='active',revision=revision+1 WHERE id=? AND org_id=? AND status='copying' AND NOT is_delete AND revision=1`, saved.TargetProjectID, saved.OrgID)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return domain.ErrProjectCopyStateConflict
		}
		if err := saveCopyJob(tx, before, saved); err != nil {
			return err
		}
		return copyChanged(tx, saved, "published", time.Now())
	})
	return saved, err
}

// Fail preserves uncertainty and cancellation after a stopped owned attempt.
func (s *ProjectCopyStore) Fail(ctx context.Context, actor identityapp.Principal, id, worker uuid.UUID, code string, retryable, unknown bool) (domain.ProjectCopyJob, error) {
	var saved domain.ProjectCopyJob
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		before, err := readCopyJob(tx, actor, id, true)
		if err != nil {
			return err
		}
		saved = before
		if err := saved.Fail(worker, code, retryable, unknown); err != nil {
			return err
		}
		if err := saveCopyJob(tx, before, saved); err != nil {
			return err
		}
		return copyChanged(tx, saved, "failed", time.Now())
	})
	return saved, err
}

// FinishCancelled commits only after byte absence and both owners' cleanup receipts.
func (s *ProjectCopyStore) FinishCancelled(ctx context.Context, actor identityapp.Principal, id, worker uuid.UUID) (domain.ProjectCopyJob, error) {
	var saved domain.ProjectCopyJob
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		before, err := readCopyJob(tx, actor, id, true)
		if err != nil {
			return err
		}
		if before.Status == "cancelled" && worker != uuid.Nil {
			saved = before
			return nil
		}
		if before.Status != "cancel_requested" || before.WorkerID != worker || worker == uuid.Nil {
			return domain.ErrProjectCopyWorkerConflict
		}
		owners, err := s.transactionOwners(tx)
		if err != nil {
			return err
		}
		if err := owners.Media.FinishCleanup(ctx, actor, copyMediaBinding(before), copyMediaSnapshot(before)); err != nil {
			return err
		}
		if err := owners.Canvas.Cleanup(ctx, actor, copyCanvasBinding(before), copyCanvasSnapshot(before)); err != nil {
			return err
		}
		saved = before
		if err := saved.ConfirmCancelled(worker, true); err != nil {
			return err
		}
		if err := retireCopyTarget(tx, before, time.Now().UTC()); err != nil {
			return err
		}
		if err := saveCopyJob(tx, before, saved); err != nil {
			return err
		}
		return copyChanged(tx, saved, "cancelled", time.Now())
	})
	return saved, err
}
