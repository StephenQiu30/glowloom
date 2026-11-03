package application

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
)

// RuntimeTransport owns the signed request protocol to the isolated Agent Host.
type RuntimeTransport interface {
	Drain(context.Context, domain.RuntimeIdentity) error
	Submit(context.Context, RuntimeDelivery) error
}

// Dispatcher sends durable commands only after their database lease commits.
type Dispatcher struct {
	store     ControlStore
	runtime   RuntimeTransport
	now       func() time.Time
	pollEvery time.Duration
}

func NewDispatcher(store ControlStore, runtime RuntimeTransport, now func() time.Time) *Dispatcher {
	return &Dispatcher{store: store, runtime: runtime, now: now, pollEvery: 250 * time.Millisecond}
}

// Run polls the PostgreSQL delivery ledger until its owning process shuts down.
func (d *Dispatcher) Run(ctx context.Context) error {
	ticker := time.NewTicker(d.pollEvery)
	defer ticker.Stop()
	for {
		worked, err := d.DispatchOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if worked {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// DispatchOnce performs at most one claimed delivery. Network outcomes are retried with the same command identity.
func (d *Dispatcher) DispatchOnce(ctx context.Context) (bool, error) {
	claim, found, err := d.store.ClaimDelivery(ctx, d.now())
	if err != nil || !found {
		return false, err
	}
	err = d.deliver(ctx, claim.Value)
	if err != nil {
		if ctx.Err() != nil {
			return true, ctx.Err()
		}
		return true, d.store.RetryDelivery(ctx, claim.ID, claim.Token, d.now().Add(retryDelay(claim.Attempt)))
	}
	return true, d.store.CompleteDelivery(ctx, claim.ID, claim.Token)
}

func (d *Dispatcher) deliver(ctx context.Context, delivery RuntimeDelivery) error {
	switch delivery.Kind {
	case "drain":
		return d.runtime.Drain(ctx, delivery.Identity)
	case "submit":
		// Drain is idempotent and closes an older approved phase before reopening the next one.
		if err := d.runtime.Drain(ctx, delivery.Identity); err != nil {
			return err
		}
		return d.runtime.Submit(ctx, delivery)
	default:
		return errors.New("invalid_agent_runtime_delivery")
	}
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	seconds := math.Min(300, 2*math.Pow(2, float64(attempt-1)))
	return time.Duration(seconds) * time.Second
}
