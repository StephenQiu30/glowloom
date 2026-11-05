package task

import (
	"context"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// Retention identifies the project retention cleanup task kind.
const Retention = "workspace.retention"

// RetentionHandler executes due project retention work through its owning service.
func RetentionHandler(service *application.ProjectRetention) taskruntime.Handler {
	return func(ctx context.Context, _ taskruntime.Job) (time.Time, error) {
		count, err := service.Run(ctx, 100)
		next := time.Now().UTC()
		if count < 100 {
			next = next.Truncate(24 * time.Hour).Add(5 * time.Hour)
			if !next.After(time.Now()) {
				next = next.Add(24 * time.Hour)
			}
		}
		return next, err
	}
}
