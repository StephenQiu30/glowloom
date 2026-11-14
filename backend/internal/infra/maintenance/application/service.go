// Package application coordinates durable infrastructure retention work.
package application

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrInvalidBatchSize means a cleanup batch cannot make progress.
var ErrInvalidBatchSize = errors.New("maintenance batch size must be positive")

type outboxStore interface {
	PrunePublished(context.Context, time.Time, int) (int64, error)
}

type processedEventStore interface {
	PruneExpired(context.Context, time.Time, int) (int64, error)
}

// Service coordinates message retention through injected DML-only store ports.
type Service struct {
	outbox    outboxStore
	processed processedEventStore
}

// NewService constructs maintenance work from the stores that own each table.
func NewService(outbox outboxStore, processed processedEventStore) *Service {
	return &Service{outbox: outbox, processed: processed}
}

// PruneOutbox drains events that have been published for over seven days.
func (s *Service) PruneOutbox(ctx context.Context, now time.Time, batchSize int, progress func(int64)) (int64, error) {
	return drainBatches(ctx, now, batchSize, s.outbox.PrunePublished, progress)
}

// PruneProcessedEvents drains consumer deduplication markers older than 30 days.
func (s *Service) PruneProcessedEvents(ctx context.Context, now time.Time, batchSize int, progress func(int64)) (int64, error) {
	return drainBatches(ctx, now, batchSize, s.processed.PruneExpired, progress)
}

func drainBatches(
	ctx context.Context,
	now time.Time,
	batchSize int,
	prune func(context.Context, time.Time, int) (int64, error),
	progress func(int64),
) (int64, error) {
	if batchSize <= 0 {
		return 0, ErrInvalidBatchSize
	}
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		removed, err := prune(ctx, now, batchSize)
		if err != nil {
			return total, fmt.Errorf("prune expired event batch: %w", err)
		}
		total += removed
		if progress != nil {
			progress(total)
		}
		if removed < int64(batchSize) {
			return total, nil
		}
	}
}
