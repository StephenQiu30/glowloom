// Package postgres records Kafka consumer effects and event IDs in one transaction.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	// ErrConsumerRequired means the consumer name is empty.
	ErrConsumerRequired = errors.New("consumer name is required")
	// ErrInvalidEventID means the event ID is not a UUID.
	ErrInvalidEventID = errors.New("invalid event ID")
	// ErrHandlerRequired means there is no effect to execute.
	ErrHandlerRequired = errors.New("event handler is required")
)

// Store owns the PostgreSQL transaction for one consumer event.
type Store struct {
	db *gorm.DB
}

// NewStore constructs an event deduplication store.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// ProcessOnce records an event and runs its effect in the same transaction.
// The handler must use tx for PostgreSQL writes. External effects may repeat if
// they succeed but the database commit fails; they must accept the event ID as
// an idempotency key or tolerate at-least-once execution.
func (s *Store) ProcessOnce(ctx context.Context, consumer, eventID string, handle func(context.Context, *gorm.DB) error) (bool, error) {
	consumer = strings.TrimSpace(consumer)
	if consumer == "" {
		return false, ErrConsumerRequired
	}
	id, err := uuid.Parse(eventID)
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrInvalidEventID, err)
	}
	if handle == nil {
		return false, ErrHandlerRequired
	}
	markerID, err := uuid.NewV7()
	if err != nil {
		return false, fmt.Errorf("create processed event ID: %w", err)
	}
	applied := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		inserted := tx.Exec(`
			INSERT INTO infra.processed_event(id, consumer, event_id)
			VALUES (?::uuid, ?, ?::uuid)
			ON CONFLICT (consumer, event_id) DO NOTHING
		`, markerID.String(), consumer, id.String())
		if inserted.Error != nil {
			return fmt.Errorf("claim event for consumer %s: %w", consumer, inserted.Error)
		}
		if inserted.RowsAffected == 0 {
			return nil
		}
		if inserted.RowsAffected != 1 {
			return fmt.Errorf("claim event for consumer %s: inserted %d rows", consumer, inserted.RowsAffected)
		}
		if err := handle(ctx, tx); err != nil {
			return fmt.Errorf("handle event for consumer %s: %w", consumer, err)
		}
		applied = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("process event %s: %w", id, err)
	}
	return applied, nil
}

// ProcessExternalOnce records an event only after an external effect succeeds.
// The effect can run twice when PostgreSQL commit fails after it succeeds.
func (s *Store) ProcessExternalOnce(ctx context.Context, consumer, eventID string, handle func(context.Context) error) (bool, error) {
	if handle == nil {
		return false, ErrHandlerRequired
	}
	return s.ProcessOnce(ctx, consumer, eventID, func(ctx context.Context, _ *gorm.DB) error {
		return handle(ctx)
	})
}
