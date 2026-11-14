// Package task runs periodic database maintenance against its persisted due time.
package task

import (
	"context"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/infra/maintenance/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// Partitions identifies retired partition work so persisted tasks can complete.
// It must not be admitted or rescheduled as periodic maintenance.
const Partitions = "maintenance.partitions"

// Outbox and Inbox identify periodic durable message pruning.
const (
	Outbox = "maintenance.outbox"
	Inbox  = "maintenance.inbox"
)

// Handlers returns retention handlers and a completion-only retired task handler.
func Handlers(service *application.Service) map[string]taskruntime.Handler {
	return map[string]taskruntime.Handler{
		Partitions: func(context.Context, taskruntime.Job) (time.Time, error) {
			// A zero cursor completes old durable tasks without DDL or another wakeup.
			return time.Time{}, nil
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
