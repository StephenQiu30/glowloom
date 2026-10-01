package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// ProjectCopyWorkID binds physical execution to one persisted copy worker.
type ProjectCopyWorkID struct{ OrgID, JobID, WorkerID uuid.UUID }

// ProjectCopyDelivery carries a committed safe command, never an object URL or snapshot.
type ProjectCopyDelivery struct {
	EventID         uuid.UUID `json:"-"`
	ActorID         uuid.UUID `json:"-"`
	OccurredAt      time.Time `json:"-"`
	OrgID           uuid.UUID `json:"-"`
	JobID           uuid.UUID `json:"copy_job_id"`
	SourceProjectID uuid.UUID `json:"source_project_id"`
	TargetProjectID uuid.UUID `json:"target_project_id"`
	Status          string    `json:"status"`
	Stage           string    `json:"stage"`
	Revision        int64     `json:"revision"`
	Action          string    `json:"action"`
}

// ProjectCopyExecutionStore persists current actor authority, attempts and owner receipts.
type ProjectCopyExecutionStore interface {
	WorkerActor(context.Context, uuid.UUID, uuid.UUID) (identityapp.Principal, error)
	Find(context.Context, identityapp.Principal, uuid.UUID) (domain.ProjectCopyJob, error)
	Claim(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, bool, time.Time) (domain.ProjectCopyJob, error)
	CompleteMedia(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (domain.ProjectCopyJob, error)
	CompleteCanvases(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (domain.ProjectCopyJob, error)
	Publish(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (domain.ProjectCopyJob, error)
	FinishCancelled(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (domain.ProjectCopyJob, error)
	Fail(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, string, bool, bool) (domain.ProjectCopyJob, error)
	Interrupt(context.Context, ProjectCopyWorkID) error
}

// ProjectCopyTransferFactory binds concrete byte access to this exact worker and phase.
type ProjectCopyTransferFactory func(job domain.ProjectCopyJob, worker uuid.UUID, cleanup bool) *mediaapp.ProjectCopyTransfer

// ProjectCopyWorker executes only frozen content and waits for actual owned byte calls to return.
type ProjectCopyWorker struct {
	store    ProjectCopyExecutionStore
	transfer ProjectCopyTransferFactory
	now      func() time.Time
}

// NewProjectCopyWorker injects durable fences and the actual private transfer consumer.
func NewProjectCopyWorker(store ProjectCopyExecutionStore, transfer ProjectCopyTransferFactory, now func() time.Time) *ProjectCopyWorker {
	return &ProjectCopyWorker{store: store, transfer: transfer, now: now}
}
func copyMediaScope(j domain.ProjectCopyJob) (mediaapp.ProjectCopyBinding, mediaapp.ProjectCopySnapshot) {
	return mediaapp.ProjectCopyBinding{JobID: j.ID, OrgID: j.OrgID, SourceProjectID: j.SourceProjectID, TargetProjectID: j.TargetProjectID}, mediaapp.ProjectCopySnapshot{ID: j.Manifest.MediaSnapshotID, ManifestSHA256: j.Manifest.MediaSHA256, Assets: j.Manifest.Assets, Renditions: j.Manifest.Renditions}
}
func (w *ProjectCopyWorker) finishFailure(ctx context.Context, actor identityapp.Principal, id ProjectCopyWorkID, cause error) (domain.ProjectCopyJob, error) {
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	current, err := w.store.Find(finish, actor, id.JobID)
	if err != nil {
		return domain.ProjectCopyJob{}, errors.Join(cause, err)
	}
	if current.Status == "succeeded" || current.Status == "cancelled" {
		return current, nil
	}
	code, unknown, retryable := "copy_failed", false, true
	var transfer *mediaapp.ProjectCopyTransferError
	if errors.As(cause, &transfer) {
		code, unknown = transfer.Code, transfer.NeedsReconciliation
		retryable = !unknown
	}
	if current.CancellationRequested && !unknown {
		code, unknown, retryable = "cleanup_requires_reconciliation", true, false
	}
	if ctx.Err() != nil || current.ExecutionUnconfirmed {
		code, unknown, retryable = "execution_stopped_after_timeout", true, false
	}
	if errors.Is(cause, mediaapp.ErrProjectCopyMediaUnavailable) || errors.Is(cause, domain.ErrInvalidProjectCopy) {
		code, retryable = "frozen_content_invalid", false
	}
	failed, err := w.store.Fail(finish, actor, id.JobID, id.WorkerID, code, retryable, unknown)
	if err != nil {
		return current, errors.Join(cause, err)
	}
	return failed, nil
}

// Execute keeps one synchronous worker until publication, verified cleanup or a persisted failure.
func (w *ProjectCopyWorker) Execute(ctx context.Context, id ProjectCopyWorkID) (domain.ProjectCopyJob, error) {
	if w == nil || w.store == nil || w.transfer == nil || w.now == nil || id.OrgID == uuid.Nil || id.JobID == uuid.Nil || id.WorkerID == uuid.Nil {
		return domain.ProjectCopyJob{}, ErrProjectDependencyUnavailable
	}
	actor, err := w.store.WorkerActor(ctx, id.OrgID, id.JobID)
	if err != nil {
		return domain.ProjectCopyJob{}, err
	}
	job, err := w.store.Find(ctx, actor, id.JobID)
	if err != nil {
		return job, err
	}
	if job.Status == "succeeded" || job.Status == "cancelled" || job.Status == "failed" && !job.ReconciliationRequested {
		return job, nil
	}
	if job.WorkerID != uuid.Nil {
		return job, domain.ErrProjectCopyWorkerConflict
	}
	job, err = w.store.Claim(ctx, actor, id.JobID, id.WorkerID, job.ReconciliationRequested, w.now().UTC())
	if err != nil {
		return job, err
	}
	for steps := 0; steps < 5; steps++ {
		if ctx.Err() != nil {
			return w.finishFailure(ctx, actor, id, ctx.Err())
		}
		current, err := w.store.Find(ctx, actor, id.JobID)
		if err != nil {
			return w.finishFailure(ctx, actor, id, err)
		}
		job = current
		if job.WorkerID != id.WorkerID {
			return job, domain.ErrProjectCopyWorkerConflict
		}
		binding, snapshot := copyMediaScope(job)
		switch job.Stage {
		case "media":
			transfer := w.transfer(job, id.WorkerID, false)
			if transfer == nil {
				return w.finishFailure(ctx, actor, id, ErrProjectDependencyUnavailable)
			}
			err = transfer.Transfer(ctx, actor, binding, snapshot)
			if err == nil {
				job, err = w.store.CompleteMedia(ctx, actor, id.JobID, id.WorkerID)
			}
		case "canvases":
			job, err = w.store.CompleteCanvases(ctx, actor, id.JobID, id.WorkerID)
		case "finalizing":
			job, err = w.store.Publish(ctx, actor, id.JobID, id.WorkerID)
		case "cleanup":
			transfer := w.transfer(job, id.WorkerID, true)
			if transfer == nil {
				return w.finishFailure(ctx, actor, id, ErrProjectDependencyUnavailable)
			}
			err = transfer.Cleanup(ctx, actor, binding, snapshot)
			if err == nil {
				job, err = w.store.FinishCancelled(ctx, actor, id.JobID, id.WorkerID)
			}
		default:
			err = domain.ErrInvalidProjectCopy
		}
		if err != nil {
			var transfer *mediaapp.ProjectCopyTransferError
			unknown := errors.As(err, &transfer) && transfer.NeedsReconciliation
			if !unknown && ctx.Err() == nil {
				current, readErr := w.store.Find(ctx, actor, id.JobID)
				if readErr == nil && current.Status == "cancel_requested" && current.WorkerID == id.WorkerID {
					continue
				}
			}
			return w.finishFailure(ctx, actor, id, err)
		}
		if job.Status == "succeeded" || job.Status == "cancelled" {
			return job, nil
		}
	}
	return w.finishFailure(ctx, actor, id, domain.ErrInvalidProjectCopy)
}
