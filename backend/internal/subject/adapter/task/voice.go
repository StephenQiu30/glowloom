// Package task executes committed voice requests against their original receipts.
package task

import (
	"context"
	"errors"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
)

// Voice identifies the subject voice generation task kind.
const Voice = "subject.voice"

// VoiceHandler executes admitted voice jobs while retaining their provider receipts.
func VoiceHandler(worker *application.VoiceWorker) taskruntime.Handler {
	run := taskruntime.WithReference(12*time.Minute, func(ctx context.Context, _ taskruntime.Job, id application.VoiceWorkID) error {
		_, err := worker.Run(ctx, id)
		return err
	})
	return func(ctx context.Context, job taskruntime.Job) (time.Time, error) {
		next, err := run(ctx, job)
		if errors.Is(err, application.ErrVoiceCapacity) {
			// Capacity is a durable wait, not an execution failure. Preserve the
			// queued intent without consuming the bounded storage-error retries.
			return time.Now().Add(5 * time.Second), nil
		}
		// Run already records uncertain sends and recovers original receipts.
		// Recording an interruption cannot stand in for successful publication.
		return next, err
	}
}
