package application

import (
	"context"
	"fmt"
)

// ProjectRetentionStore owns durable expired deletion jobs and locked cleanup.
type ProjectRetentionStore interface {
	EnqueueExpired(context.Context, int) (int64, error)
	ProcessNext(context.Context) (bool, error)
}

// ProjectRetention drains bounded batches through the owning module store.
type ProjectRetention struct{ store ProjectRetentionStore }

// NewProjectRetention injects the worker store; no request can invoke this use case.
func NewProjectRetention(store ProjectRetentionStore) *ProjectRetention {
	return &ProjectRetention{store: store}
}

// Run performs bounded work so cancellation and retries remain under workflow ownership.
func (s *ProjectRetention) Run(ctx context.Context, limit int) (int, error) {
	if s == nil || s.store == nil || limit < 1 || limit > 500 {
		return 0, ErrProjectDependencyUnavailable
	}
	if _, err := s.store.EnqueueExpired(ctx, limit); err != nil {
		return 0, fmt.Errorf("queue project retention: %w", err)
	}
	for n := 0; n < limit; n++ {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		worked, err := s.store.ProcessNext(ctx)
		if err != nil {
			return n, fmt.Errorf("process project retention: %w", err)
		}
		if !worked {
			return n, nil
		}
	}
	return limit, nil
}
