// Package application coordinates durable Outbox delivery.
package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	deliveryTimeout     = 10 * time.Second
	claimLease          = 30 * time.Second
	idlePollInterval    = 250 * time.Millisecond
	maxDeliveryAttempts = 12
	retryBase           = time.Second
	retryCap            = 5 * time.Minute
)

// ErrInvalidEnvelope means an Outbox event cannot be safely published.
var ErrInvalidEnvelope = errors.New("invalid outbox event envelope")

// Event is a claimed Outbox message with its current delivery lease.
type Event struct {
	ID           string
	Topic        string
	PartitionKey string
	Payload      json.RawMessage
	Headers      map[string]string
	CreateTime   time.Time
	ClaimToken   string
	AttemptCount int
}

// Store persists Outbox leases, acknowledgements, retries, and dead letters.
type Store interface {
	ClaimNext(context.Context, time.Time, time.Duration) (Event, bool, error)
	Acknowledge(context.Context, Event, time.Time) error
	Retry(context.Context, Event, string, int, time.Time, time.Time) (bool, error)
}

// Publisher sends one committed Outbox event to the message broker.
type Publisher interface {
	Publish(context.Context, Event) error
}

// Relay publishes durable Outbox events and schedules bounded retries.
type Relay struct {
	store     Store
	publisher Publisher
}

// NewRelay constructs a relay with its durable store and broker publisher.
func NewRelay(store Store, publisher Publisher) *Relay {
	return &Relay{store: store, publisher: publisher}
}

// RunOnce claims and processes at most one event.
func (r *Relay) RunOnce(ctx context.Context) (bool, error) {
	now := time.Now().UTC()
	event, claimed, err := r.store.ClaimNext(ctx, now, claimLease)
	if err != nil {
		return false, fmt.Errorf("claim Outbox event: %w", err)
	}
	if !claimed {
		return false, nil
	}
	deliveryCtx, cancel := context.WithTimeout(ctx, deliveryTimeout)
	defer cancel()
	if err := event.validate(); err != nil {
		attempts := max(event.AttemptCount, 1)
		if _, retryErr := r.store.Retry(ctx, event, "invalid_envelope", attempts, retryAt(now, event.AttemptCount), now); retryErr != nil {
			return false, fmt.Errorf("dead-letter invalid Outbox event: %w", retryErr)
		}
		return false, nil
	}
	if err := r.publisher.Publish(deliveryCtx, event); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		if _, retryErr := r.store.Retry(ctx, event, deliveryErrorClass(err), maxDeliveryAttempts, retryAt(now, event.AttemptCount), now); retryErr != nil {
			return false, fmt.Errorf("schedule failed Outbox delivery: %w", retryErr)
		}
		return false, nil
	}
	if err := r.store.Acknowledge(ctx, event, time.Now().UTC()); err != nil {
		return false, fmt.Errorf("acknowledge delivered Outbox event: %w", err)
	}
	return true, nil
}

// Run polls and publishes events until the context is canceled.
func (r *Relay) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		sent, err := r.RunOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("run Outbox relay: %w", err)
		}
		if sent {
			continue
		}
		timer := time.NewTimer(idlePollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func retryAt(now time.Time, attempt int) time.Time {
	if attempt < 1 {
		attempt = 1
	}
	shift := min(attempt-1, 20)
	delay := retryBase * time.Duration(1<<shift)
	if delay > retryCap || delay < 0 {
		delay = retryCap
	}
	return now.Add(delay)
}

func deliveryErrorClass(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "broker_timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "delivery_canceled"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "timeout") || strings.Contains(message, "deadline"):
		return "broker_timeout"
	case strings.Contains(message, "authorization") || strings.Contains(message, "permission"):
		return "broker_authorization"
	default:
		return "broker_publish_failed"
	}
}

func (event Event) validate() error {
	if event.ID == "" || event.Topic == "" || event.PartitionKey == "" {
		return fmt.Errorf("%w: missing routing field", ErrInvalidEnvelope)
	}
	var envelope struct {
		EventID   string `json:"event_id"`
		EventType string `json:"event_type"`
	}
	if err := json.Unmarshal(event.Payload, &envelope); err != nil {
		return fmt.Errorf("%w: decode body: %w", ErrInvalidEnvelope, err)
	}
	if envelope.EventID != event.ID || envelope.EventType != event.Topic {
		return fmt.Errorf("%w: body does not match row", ErrInvalidEnvelope)
	}
	return nil
}
