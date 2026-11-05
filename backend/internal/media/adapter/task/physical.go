// Package task executes private transfer and purge work against the original attempts.
package task

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// Physical media task kinds identify transfer and purge execution.
const (
	Transfer = "media.transfer"
	Purge    = "media.purge"
)

func execution(j taskruntime.Job, epoch int64) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(j.Kind+"/"+j.BusinessID+"/"+strconv.FormatInt(epoch, 10))).String()
}

// TransferStore owns the durable transfer delivery and fenced worker operations.
type TransferStore interface {
	InterruptTransfer(context.Context, application.TransferWorkID) error
}

// TransferHandler executes an admitted transfer using its original task reference.
func TransferHandler(worker *application.TransferWorker, store TransferStore) taskruntime.Handler {
	return taskruntime.WithReference(2*time.Hour, func(ctx context.Context, j taskruntime.Job, id application.TransferWorkID) error {
		if j.Epoch > 1 {
			id.ExecutionID = execution(j, j.Epoch-1)
			if err := store.InterruptTransfer(ctx, id); err != nil && !errors.Is(err, domain.ErrTransferConflict) {
				return err
			}
		}
		id.ExecutionID = execution(j, j.Epoch)
		_, err := worker.Execute(ctx, id)
		if err != nil && ctx.Err() == nil {
			if mark := store.InterruptTransfer(ctx, id); mark != nil && !errors.Is(mark, domain.ErrTransferConflict) {
				return mark
			}
		}
		return err
	})
}

// PurgeStore owns the durable purge delivery and fenced worker operations.
type PurgeStore interface {
	InterruptPurge(context.Context, application.PurgeWorkID) error
}

// PurgeHandler executes an admitted purge using its original task reference.
func PurgeHandler(worker *application.PurgeWorker, store PurgeStore) taskruntime.Handler {
	return taskruntime.WithReference(2*time.Hour, func(ctx context.Context, j taskruntime.Job, id application.PurgeWorkID) error {
		if j.Epoch > 1 {
			id.ExecutionID = execution(j, j.Epoch-1)
			if err := store.InterruptPurge(ctx, id); err != nil && !errors.Is(err, domain.ErrPurgeConflict) {
				return err
			}
		}
		id.ExecutionID = execution(j, j.Epoch)
		_, err := worker.Execute(ctx, id)
		if err != nil && ctx.Err() == nil {
			if mark := store.InterruptPurge(ctx, id); mark != nil && !errors.Is(mark, domain.ErrPurgeConflict) {
				return mark
			}
		}
		return err
	})
}
