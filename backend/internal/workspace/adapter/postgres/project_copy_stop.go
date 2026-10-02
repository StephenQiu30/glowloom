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

// StopAttempt is called only after this worker's synchronous byte calls have returned.
// Revoked user authority cannot prevent exact-worker cessation; this never accesses owners.
func (s *ProjectCopyStore) StopAttempt(ctx context.Context, id application.ProjectCopyWorkID, code string, retryable, unknown bool) (domain.ProjectCopyJob, error) {
	if s == nil || s.db == nil || id.OrgID == uuid.Nil || id.JobID == uuid.Nil || id.WorkerID == uuid.Nil {
		return domain.ProjectCopyJob{}, domain.ErrInvalidProjectCopy
	}
	var stopped domain.ProjectCopyJob
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		before, err := readCopyJob(tx, identityapp.Principal{OrgID: id.OrgID}, id.JobID, true)
		if err != nil {
			return err
		}
		if before.WorkerID != id.WorkerID {
			return domain.ErrProjectCopyWorkerConflict
		}
		if before.ExecutionUnconfirmed {
			code, unknown, retryable = "execution_stopped_after_timeout", true, false
		} else if before.CancellationRequested && !unknown {
			code, unknown, retryable = "cleanup_requires_reconciliation", true, false
		}
		stopped = before
		if err := stopped.Fail(id.WorkerID, code, retryable, unknown); err != nil {
			return err
		}
		if err := saveCopyJob(tx, before, stopped); err != nil {
			return err
		}
		return copyChanged(tx, stopped, "failed", time.Now())
	})
	return stopped, err
}
