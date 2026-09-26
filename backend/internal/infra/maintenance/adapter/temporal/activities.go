package temporal

import (
	"context"
	"time"

	"go.temporal.io/sdk/activity"

	"github.com/StephenQiu30/lanverse/backend/internal/infra/maintenance/application"
)

// Activities adapts maintenance use cases to the Temporal flow queue.
type Activities struct {
	service   *application.Service
	batchSize int
}

// NewActivities injects the database maintenance service and cleanup batch size.
func NewActivities(service *application.Service, batchSize int) *Activities {
	return &Activities{service: service, batchSize: batchSize}
}

// MaintainPartitions prepares and safely retires Outbox month partitions.
func (a *Activities) MaintainPartitions(ctx context.Context) (int, error) {
	return a.service.MaintainPartitions(ctx, time.Now().UTC())
}

// PruneOutbox removes expired published events in bounded database batches.
func (a *Activities) PruneOutbox(ctx context.Context) (int64, error) {
	return a.service.PruneOutbox(ctx, time.Now().UTC(), a.batchSize, func(total int64) {
		activity.RecordHeartbeat(ctx, total)
	})
}

// PruneProcessedEvents removes expired consumer markers in bounded batches.
func (a *Activities) PruneProcessedEvents(ctx context.Context) (int64, error) {
	return a.service.PruneProcessedEvents(ctx, time.Now().UTC(), a.batchSize, func(total int64) {
		activity.RecordHeartbeat(ctx, total)
	})
}
