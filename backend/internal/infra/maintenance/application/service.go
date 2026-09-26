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
	EnsurePartitions(context.Context, time.Time) error
	DropEmptyPastPartitions(context.Context, time.Time) (int, error)
	PrunePublished(context.Context, time.Time, int) (int64, error)
}

type processedEventStore interface {
	PruneExpired(context.Context, time.Time, int) (int64, error)
}

// Service coordinates Outbox partitions and event retention through injected stores.
type Service struct {
	outbox    outboxStore
	processed processedEventStore
}

// NewService constructs maintenance work from the stores that own each table.
func NewService(outbox outboxStore, processed processedEventStore) *Service {
	return &Service{outbox: outbox, processed: processed}
}

// MaintainPartitions prepares writable Outbox partitions, then removes only
// completed month partitions that are empty.
func (s *Service) MaintainPartitions(ctx context.Context, now time.Time) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := s.outbox.EnsurePartitions(ctx, now); err != nil {
		return 0, fmt.Errorf("ensure Outbox partitions: %w", err)
	}
	dropped, err := s.outbox.DropEmptyPastPartitions(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("drop empty Outbox partitions: %w", err)
	}
	return dropped, nil
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
