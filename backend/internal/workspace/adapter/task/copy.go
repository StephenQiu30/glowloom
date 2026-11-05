// Package task consumes frozen project-copy attempts and their permanent controls.
package task

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

// Copy identifies the project copy task kind.
const Copy = "workspace.copy"

// CopyHandler executes admitted project copy stages using their original task references.
func CopyHandler(worker *application.ProjectCopyWorker, store application.ProjectCopyExecutionStore) taskruntime.Handler {
	return taskruntime.WithReference(2*time.Hour, func(ctx context.Context, j taskruntime.Job, id application.ProjectCopyWorkID) error {
		exec := func(epoch int64) uuid.UUID {
			return uuid.NewSHA1(id.JobID, []byte(j.BusinessID+"/"+strconv.FormatInt(epoch, 10)))
		}
		if j.Epoch > 1 {
			id.WorkerID = exec(j.Epoch - 1)
			if err := store.Interrupt(ctx, id); err != nil && !errors.Is(err, domain.ErrProjectCopyWorkerConflict) {
				return err
			}
		}
		id.WorkerID = exec(j.Epoch)
		_, err := worker.Execute(ctx, id)
		if err != nil && ctx.Err() == nil {
			if mark := store.Interrupt(ctx, id); mark != nil && !errors.Is(mark, domain.ErrProjectCopyWorkerConflict) {
				return mark
			}
		}
		return err
	})
}
