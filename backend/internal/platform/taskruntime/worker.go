package taskruntime

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

// Handler executes one bounded business step. A zero time closes the execution
// window. A future time durably waits for the next step without occupying a slot.
type Handler func(context.Context, Job) (time.Time, error)

// Worker claims bounded business steps and joins their lease renewal on exit.
type Worker struct {
	Store       *Store
	Handlers    map[string]Handler
	Concurrency int
	Lease       time.Duration
	Poll        time.Duration
}

// Run processes configured task kinds until cancellation or a worker failure.
func (w *Worker) Run(ctx context.Context) error {
	if w.Store == nil || len(w.Handlers) == 0 || w.Concurrency < 1 || w.Lease < 3*time.Millisecond || w.Poll <= 0 {
		return errors.New("invalid task worker configuration")
	}
	kinds := slices.Sorted(maps.Keys(w.Handlers))
	for _, kind := range kinds {
		if kind == "" || w.Handlers[kind] == nil {
			return errors.New("invalid task handler")
		}
	}
	group, owned := errgroup.WithContext(ctx)
	for range w.Concurrency {
		group.Go(func() error {
			owner := uuid.NewString()
			tick := time.NewTicker(w.Poll)
			defer tick.Stop()
			for owned.Err() == nil {
				j, err := w.Store.Claim(owned, kinds, owner, w.Lease)
				if err != nil {
					if owned.Err() != nil {
						return nil
					}
					return err
				}
				if j == nil {
					select {
					case <-owned.Done():
						return nil
					case <-tick.C:
					}
					continue
				}
				if err = w.execute(owned, *j); err != nil && !errors.Is(err, ErrLeaseLost) && owned.Err() == nil {
					return err
				}
			}
			return nil
		})
	}
	return group.Wait()
}
func (w *Worker) execute(ctx context.Context, j Job) error {
	run, cancel := context.WithCancelCause(ctx)
	renewal, stopRenewal := context.WithCancel(ctx)
	renew := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(w.Lease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-renewal.Done():
				renew <- nil
				return
			case <-ticker.C:
				if err := w.Store.Renew(renewal, j, w.Lease); err != nil {
					if errors.Is(err, ErrStopRequested) {
						cancel(ErrStopRequested)
						continue
					}
					cancel(err)
					renew <- err
					return
				}
			}
		}
	}()
	next, err := w.Handlers[j.Kind](context.WithValue(run, claimKey{}, j), j)
	cancel(nil)
	stopRenewal()
	if leaseErr := <-renew; leaseErr != nil && !errors.Is(leaseErr, context.Canceled) {
		return leaseErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		// Business adapters persist classified terminal failures themselves. Storage
		// failures and interrupted steps keep their original identity for recovery.
		return w.Store.retry(ctx, j)
	}
	if !next.IsZero() {
		if rescheduleErr := w.Store.Reschedule(ctx, j, next); rescheduleErr != nil {
			return fmt.Errorf("reschedule %s: %w", j.Kind, rescheduleErr)
		}
		return nil
	}
	return w.Store.Complete(ctx, j)
}
