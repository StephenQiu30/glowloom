// Package task executes saved file import commands against the exact source revision.
package task

import (
	"context"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
)

// Import identifies the script import task kind.
const Import = "script.import"

// ImportHandler executes admitted script imports using their original task references.
func ImportHandler(worker *application.ImportWorker, store application.ImportWorkerStore) taskruntime.Handler {
	return taskruntime.WithReference(30*time.Minute, func(ctx context.Context, _ taskruntime.Job, d application.ImportDelivery) error {
		_, err := worker.Run(ctx, d)
		if err != nil && ctx.Err() == nil {
			return store.FailImportWorkflow(ctx, d.ImportWork, time.Now().UTC())
		}
		return err
	})
}
