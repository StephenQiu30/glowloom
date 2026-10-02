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

func (s *ProjectCopyStore) bibleOwner(tx *gorm.DB, job domain.ProjectCopyJob, worker uuid.UUID, phase string) (application.ProjectCopyBibleOwner, error) {
	if s.bible == nil {
		return nil, application.ErrProjectDependencyUnavailable
	}
	owner := s.bible(tx, application.ProjectCopyAuthority{Binding: copyBinding(job), ActorID: job.ExecutionActor(), WorkerID: worker, SourceRevision: job.SourceRevision, Phase: bibleCopyPhase(phase), Bible: job.Manifest.Bible}, copyMediaSnapshot(job))
	if owner == nil {
		return nil, application.ErrProjectDependencyUnavailable
	}
	return owner, nil
}

// CompleteBible registers the full Bible history and advances the same worker atomically.
func (s *ProjectCopyStore) CompleteBible(ctx context.Context, actor identityapp.Principal, id, worker uuid.UUID) (domain.ProjectCopyJob, error) {
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
		if before.Status != "running" || before.Stage != "bible" || before.CancellationRequested || before.Manifest.Bible == nil {
			return domain.ErrProjectCopyStateConflict
		}
		owner, err := s.bibleOwner(tx, before, worker, "register")
		if err != nil {
			return err
		}
		receipt, err := owner.Register(ctx, actor, copyBinding(before), *before.Manifest.Bible)
		if err != nil {
			return err
		}
		saved = before
		if err := saved.AcceptBibleReceipt(worker, receipt); err != nil {
			return err
		}
		if err := saveCopyJob(tx, before, saved); err != nil {
			return err
		}
		return copyChanged(tx, saved, "checkpoint", time.Now())
	})
	return saved, err
}

func (s *ProjectCopyStore) verifyBible(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, job domain.ProjectCopyJob, worker uuid.UUID) error {
	if job.Manifest.Bible == nil {
		return nil
	}
	owner, err := s.bibleOwner(tx, job, worker, "register")
	if err != nil {
		return err
	}
	receipt, err := owner.Verify(ctx, actor, copyBinding(job), *job.Manifest.Bible)
	if err != nil {
		return err
	}
	if job.BibleReceipt == nil || receipt != *job.BibleReceipt {
		return domain.ErrInvalidProjectCopy
	}
	return nil
}

func (s *ProjectCopyStore) finishBibleCleanup(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, job domain.ProjectCopyJob, worker uuid.UUID) error {
	if job.Manifest.Bible == nil {
		return nil
	}
	owner, err := s.bibleOwner(tx, job, worker, "cleanup")
	if err != nil {
		return err
	}
	return owner.FinishCleanup(ctx, actor, copyBinding(job), *job.Manifest.Bible)
}

func bibleCopyPhase(phase string) string {
	switch phase {
	case "transfer", "register", "verify":
		return "bible_" + phase
	default:
		return phase
	}
}
