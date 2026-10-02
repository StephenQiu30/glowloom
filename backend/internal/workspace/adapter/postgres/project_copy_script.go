package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func copyBinding(job domain.ProjectCopyJob) application.ProjectCopyBinding {
	return application.ProjectCopyBinding{JobID: job.ID, OrgID: job.OrgID, SourceProjectID: job.SourceProjectID, TargetProjectID: job.TargetProjectID}
}

func (s *ProjectCopyStore) scriptOwner(tx *gorm.DB, job domain.ProjectCopyJob, worker uuid.UUID, phase string) (application.ProjectCopyScriptOwner, error) {
	if s.script == nil {
		return nil, application.ErrProjectDependencyUnavailable
	}
	owner := s.script(tx, application.ProjectCopyAuthority{Binding: copyBinding(job), ActorID: job.ExecutionActor(), WorkerID: worker, SourceRevision: job.SourceRevision, Phase: phase, Bible: job.Manifest.Bible})
	if owner == nil {
		return nil, application.ErrProjectDependencyUnavailable
	}
	return owner, nil
}

// CompleteScript registers the full target history and advances the same worker atomically.
func (s *ProjectCopyStore) CompleteScript(ctx context.Context, actor identityapp.Principal, id, worker uuid.UUID) (domain.ProjectCopyJob, error) {
	if s == nil || s.db == nil {
		return domain.ProjectCopyJob{}, application.ErrProjectDependencyUnavailable
	}
	var saved domain.ProjectCopyJob
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		before, err := readCopyJob(tx, actor, id, true)
		if err != nil {
			return err
		}
		if before.ExecutionActor() != actor.ID {
			return identityapp.ErrForbidden
		}
		if worker == uuid.Nil || before.WorkerID != worker {
			return domain.ErrProjectCopyWorkerConflict
		}
		if before.Status != "running" || before.Stage != "script" || before.CancellationRequested || before.Manifest.Script == nil {
			return domain.ErrProjectCopyStateConflict
		}
		owner, err := s.scriptOwner(tx, before, worker, "register")
		if err != nil {
			return err
		}
		receipt, err := owner.Register(ctx, actor, copyBinding(before), *before.Manifest.Script)
		if err != nil {
			return err
		}
		saved = before
		if err := saved.AcceptScriptReceipt(worker, receipt); err != nil {
			return err
		}
		if err := saveCopyJob(tx, before, saved); err != nil {
			return err
		}
		return copyChanged(tx, saved, "checkpoint", time.Now())
	})
	return saved, err
}

func (s *ProjectCopyStore) verifyScript(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, job domain.ProjectCopyJob, worker uuid.UUID) error {
	if job.Manifest.Script == nil {
		return nil
	}
	owner, err := s.scriptOwner(tx, job, worker, "register")
	if err != nil {
		return err
	}
	receipt, err := owner.Register(ctx, actor, copyBinding(job), *job.Manifest.Script)
	if err != nil {
		return err
	}
	if job.ScriptReceipt == nil || receipt != *job.ScriptReceipt {
		return domain.ErrInvalidProjectCopy
	}
	return nil
}

func (s *ProjectCopyStore) finishScriptCleanup(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, job domain.ProjectCopyJob, worker uuid.UUID) error {
	if job.Manifest.Script == nil {
		return nil
	}
	owner, err := s.scriptOwner(tx, job, worker, "cleanup")
	if err != nil {
		return err
	}
	return owner.FinishCleanup(ctx, actor, copyBinding(job), *job.Manifest.Script)
}
