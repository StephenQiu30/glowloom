package task

import (
	"context"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// QuoteExpiryHandler uses the persisted due time and drains all due business
// quotes with one cutoff; restarting a worker does not reset that cutoff.
func QuoteExpiryHandler(service *application.QuoteExpiryService) taskruntime.Handler {
	return func(ctx context.Context, _ taskruntime.Job) (time.Time, error) {
		_, err := service.Run(ctx, time.Now().UTC(), nil)
		return time.Now().Add(5 * time.Minute), err
	}
}
