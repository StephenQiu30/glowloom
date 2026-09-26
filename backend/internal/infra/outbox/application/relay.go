// Package application coordinates durable Outbox delivery.
package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	deliveryTimeout  = 10 * time.Second
	idlePollInterval = 250 * time.Millisecond
)

// ErrInvalidEnvelope means the event body cannot be matched to its Outbox row.
var ErrInvalidEnvelope = errors.New("invalid outbox event envelope")

// Event is a committed Outbox row to publish to Kafka.
type Event struct {
	ID           string
	Topic        string
	PartitionKey string
	Payload      json.RawMessage
	Headers      map[string]string
	CreateTime   time.Time
}

// Store owns the transaction that claims and acknowledges one pending event.
// It acknowledges only after deliver succeeds.
type Store interface {
	WithNext(context.Context, func(Event) error) (bool, error)
}

// Publisher sends a committed event to the message broker.
type Publisher interface {
	Publish(context.Context, Event) error
}

// Relay joins the PostgreSQL claim with Kafka publication.
type Relay struct {
	store     Store
	publisher Publisher
}

// NewRelay constructs a relay with explicit storage and broker dependencies.
func NewRelay(store Store, publisher Publisher) *Relay {
	return &Relay{store: store, publisher: publisher}
}

// RunOnce publishes at most one event. A failed publish leaves the row pending.
func (r *Relay) RunOnce(ctx context.Context) (bool, error) {
	deliveryCtx, cancel := context.WithTimeout(ctx, deliveryTimeout)
	defer cancel()

	sent, err := r.store.WithNext(deliveryCtx, func(event Event) error {
		if err := event.validate(); err != nil {
			return err
		}
		return r.publisher.Publish(deliveryCtx, event)
	})
	if err != nil {
		return false, fmt.Errorf("deliver outbox event: %w", err)
	}
	return sent, nil
}

// Run drains pending events, waits when idle, and exits on cancellation.
// A delivery failure is returned so the process supervisor can report it.
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
			return fmt.Errorf("run outbox relay: %w", err)
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
