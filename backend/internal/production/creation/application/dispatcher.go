package application

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
)

type DeliveryStore interface {
	Claim(context.Context, time.Time, time.Duration) (domain.Delivery, error)
	AuthorizeDelivery(context.Context, domain.Run) error
	Complete(context.Context, domain.Delivery, string, string, *domain.Acceptance, time.Time, time.Time) error
}
type Agent interface {
	Lookup(context.Context, domain.Run) (domain.Acceptance, error)
	Accept(context.Context, domain.Run) (domain.Acceptance, error)
}
type Dispatcher struct {
	store DeliveryStore
	agent Agent
	now   func() time.Time
}

func NewDispatcher(store DeliveryStore, agent Agent, now func() time.Time) *Dispatcher {
	return &Dispatcher{store: store, agent: agent, now: now}
}

func (worker *Dispatcher) DispatchOne(ctx context.Context) error {
	delivery, err := worker.store.Claim(ctx, worker.now().UTC(), 45*time.Second)
	if err != nil {
		return err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	receipt, err := worker.agent.Lookup(requestCtx, delivery.Run)
	if errors.Is(err, ErrNotFound) {
		if authErr := worker.store.AuthorizeDelivery(requestCtx, delivery.Run); authErr != nil {
			var problem *Error
			if errors.As(authErr, &problem) && (problem.Status == 401 || problem.Status == 403 || problem.Status == 404) {
				return worker.finish(ctx, delivery, domain.Blocked, "permission_revoked", nil)
			}
			return worker.finish(ctx, delivery, domain.Unknown, "authorization_unavailable", nil)
		}
		receipt, err = worker.agent.Accept(requestCtx, delivery.Run)
	}
	if err == nil {
		err = ValidateAcceptance(delivery.Run, receipt)
	}
	if err != nil {
		var problem *Error
		if errors.As(err, &problem) && problem.Status >= 400 && problem.Status < 500 && problem.Status != 408 && problem.Status != 429 {
			code := "agent_rejected"
			switch problem.Status {
			case 401, 403:
				code = "agent_authorization_rejected"
			case 404:
				code = "agent_route_unavailable"
			case 409:
				code = "agent_command_conflict"
			case 400, 422:
				code = "agent_command_invalid"
			}
			return worker.finish(ctx, delivery, domain.Blocked, code, nil)
		}
		return worker.finish(ctx, delivery, domain.Unknown, "agent_outcome_unknown", nil)
	}
	return worker.finish(ctx, delivery, domain.Accepted, "", &receipt)
}
func (worker *Dispatcher) finish(ctx context.Context, delivery domain.Delivery, status, code string, receipt *domain.Acceptance) error {
	now := worker.now().UTC()
	delay := time.Second << min(max(delivery.Attempts, 1), 8)
	return worker.store.Complete(ctx, delivery, status, code, receipt, now, now.Add(delay))
}
func (worker *Dispatcher) Run(ctx context.Context, logger *slog.Logger) {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if err := worker.DispatchOne(ctx); err != nil && !errors.Is(err, ErrNoDelivery) && !errors.Is(err, ErrLeaseLost) && ctx.Err() == nil {
			logger.Error("creation command delivery failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}
