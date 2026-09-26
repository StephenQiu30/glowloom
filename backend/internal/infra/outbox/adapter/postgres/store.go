// Package postgres claims and acknowledges committed Outbox events.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/application"
)

// Store keeps a row locked until its broker publication is acknowledged.
type Store struct {
	db *gorm.DB
}

// NewStore constructs a PostgreSQL Outbox store.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// WithNext claims one pending event. A delivery error rolls back the claim.
// A broker acknowledgement followed by a failed database commit can result in
// a duplicate publish; consumers must deduplicate using the envelope event ID.
func (s *Store) WithNext(ctx context.Context, deliver func(application.Event) error) (bool, error) {
	sent := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var event application.Event
		var payload, headers []byte
		row := tx.Raw(`
			SELECT id::text, topic, partition_key, payload, headers, create_time
			FROM infra.outbox
			WHERE published_at IS NULL AND NOT is_delete
			ORDER BY create_time, id
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		`).Row()
		if err := row.Scan(&event.ID, &event.Topic, &event.PartitionKey, &payload, &headers, &event.CreateTime); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("claim pending outbox event: %w", err)
		}
		event.Payload = json.RawMessage(payload)
		if err := json.Unmarshal(headers, &event.Headers); err != nil {
			return fmt.Errorf("decode headers for outbox event %s: %w", event.ID, err)
		}
		if err := deliver(event); err != nil {
			return fmt.Errorf("publish outbox event %s: %w", event.ID, err)
		}

		updated := tx.Exec(`
			UPDATE infra.outbox
			SET published_at = now(), update_time = now()
			WHERE id = ?::uuid AND create_time = ? AND published_at IS NULL
		`, event.ID, event.CreateTime)
		if updated.Error != nil {
			return fmt.Errorf("acknowledge outbox event %s: %w", event.ID, updated.Error)
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("acknowledge outbox event %s: updated %d rows", event.ID, updated.RowsAffected)
		}
		sent = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("process pending outbox event: %w", err)
	}
	return sent, nil
}

var _ application.Store = (*Store)(nil)
