// Package task runs periodic database maintenance against its persisted due time.
package task

import (
	"context"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/infra/maintenance/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// Maintenance task kinds identify partition upkeep and durable message pruning.
const (
	Partitions = "maintenance.partitions"
	Outbox     = "maintenance.outbox"
	Inbox      = "maintenance.inbox"
)

// Handlers returns the persisted periodic maintenance task handlers.
func Handlers(service *application.Service) map[string]taskruntime.Handler {
	return map[string]taskruntime.Handler{
		Partitions: func(ctx context.Context, _ taskruntime.Job) (time.Time, error) {
			_, err := service.MaintainPartitions(ctx, time.Now().UTC())
			return nextUTC(3 * time.Hour), err
		},
		Outbox: func(ctx context.Context, _ taskruntime.Job) (time.Time, error) {
			_, err := service.PruneOutbox(ctx, time.Now().UTC(), 500, nil)
			return nextUTC(4 * time.Hour), err
		},
		Inbox: func(ctx context.Context, _ taskruntime.Job) (time.Time, error) {
			_, err := service.PruneProcessedEvents(ctx, time.Now().UTC(), 500, nil)
			return nextUTC(4*time.Hour + 30*time.Minute), err
		},
	}
}
func nextUTC(offset time.Duration) time.Time {
	now := time.Now().UTC()
	at := now.Truncate(24 * time.Hour).Add(offset)
	if !at.After(now) {
		at = at.Add(24 * time.Hour)
	}
	return at
}
