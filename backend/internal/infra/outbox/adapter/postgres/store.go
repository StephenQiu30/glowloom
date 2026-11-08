// Package postgres claims and acknowledges committed Outbox events.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/infra/outbox/application"
)

// ErrLeaseLost means another relay claimed or completed the event.
var ErrLeaseLost = errors.New("outbox delivery lease lost")

// Store persists Outbox delivery state and dead-letter audit facts.
type Store struct{ db *gorm.DB }

// NewStore constructs a PostgreSQL Outbox store.
func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

// ClaimNext leases one event, committing the lease before broker I/O.
func (s *Store) ClaimNext(ctx context.Context, now time.Time, lease time.Duration) (application.Event, bool, error) {
	if lease <= 0 {
		return application.Event{}, false, fmt.Errorf("outbox lease must be positive")
	}
	token, err := uuid.NewV7()
	if err != nil {
		return application.Event{}, false, fmt.Errorf("create outbox lease token: %w", err)
	}
	var event application.Event
	var payload, headers []byte
	var found bool
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := tx.Raw(`WITH candidate AS (
			SELECT id,create_time FROM infra.outbox
			WHERE published_at IS NULL AND dead_lettered_at IS NULL AND next_attempt_at<=?
			  AND (lease_until IS NULL OR lease_until<=?) AND NOT is_delete
			ORDER BY next_attempt_at,create_time,id
			LIMIT 1 FOR UPDATE SKIP LOCKED
		)
		UPDATE infra.outbox AS event
		SET attempt_count=event.attempt_count+1,claim_token=?::uuid,lease_until=?,update_time=?
		FROM candidate
		WHERE event.id=candidate.id AND event.create_time=candidate.create_time
		RETURNING event.id::text,event.topic,event.partition_key,event.payload,event.headers,event.create_time,event.attempt_count,event.claim_token::text`,
			now.UTC(), now.UTC(), token, now.UTC().Add(lease), now.UTC()).Row()
		if err := row.Scan(&event.ID, &event.Topic, &event.PartitionKey, &payload, &headers, &event.CreateTime, &event.AttemptCount, &event.ClaimToken); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("claim pending Outbox event: %w", err)
		}
		found = true
		event.Payload = json.RawMessage(payload)
		if err := json.Unmarshal(headers, &event.Headers); err != nil {
			return fmt.Errorf("decode Outbox headers: %w", err)
		}
		return nil
	})
	if err != nil {
		return application.Event{}, false, fmt.Errorf("claim pending Outbox event: %w", err)
	}
	return event, found, nil
}

// Acknowledge marks a claimed event published when the lease still belongs to this relay.
func (s *Store) Acknowledge(ctx context.Context, event application.Event, now time.Time) error {
	updated := s.db.WithContext(ctx).Exec(`UPDATE infra.outbox
		SET published_at=?,claim_token=NULL,lease_until=NULL,update_time=?
		WHERE id=?::uuid AND create_time=? AND claim_token=?::uuid AND published_at IS NULL AND dead_lettered_at IS NULL`,
		now.UTC(), now.UTC(), event.ID, event.CreateTime.UTC(), event.ClaimToken)
	if updated.Error != nil {
		return fmt.Errorf("acknowledge Outbox event %s: %w", event.ID, updated.Error)
	}
	if updated.RowsAffected != 1 {
		return fmt.Errorf("acknowledge Outbox event %s: %w", event.ID, ErrLeaseLost)
	}
	return nil
}

// Retry records a bounded retry or moves the event into the durable dead-letter table.
func (s *Store) Retry(ctx context.Context, event application.Event, errorClass string, maxAttempts int, nextAttempt time.Time, now time.Time) (bool, error) {
	if maxAttempts < 1 || errorClass == "" {
		return false, fmt.Errorf("invalid Outbox retry policy")
	}
	dead := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current struct{ AttemptCount int }
		row := tx.Raw(`SELECT attempt_count FROM infra.outbox
			WHERE id=?::uuid AND create_time=? AND claim_token=?::uuid AND published_at IS NULL AND dead_lettered_at IS NULL FOR UPDATE`,
			event.ID, event.CreateTime.UTC(), event.ClaimToken).Scan(&current)
		if row.Error != nil {
			return fmt.Errorf("lock failed Outbox event: %w", row.Error)
		}
		if row.RowsAffected != 1 {
			return ErrLeaseLost
		}
		if current.AttemptCount >= maxAttempts {
			inserted := tx.Exec(`INSERT INTO infra.outbox_dead_letter
				(event_id,event_create_time,topic,partition_key,payload,headers,attempt_count,error_class,failed_at)
				SELECT id,create_time,topic,partition_key,payload,headers,attempt_count,?,?
				FROM infra.outbox WHERE id=?::uuid AND create_time=?
				ON CONFLICT (event_id,event_create_time) DO UPDATE SET
					topic=excluded.topic,partition_key=excluded.partition_key,payload=excluded.payload,
					headers=excluded.headers,attempt_count=excluded.attempt_count,error_class=excluded.error_class,
					failed_at=excluded.failed_at,replayed_at=NULL`, errorClass, now.UTC(), event.ID, event.CreateTime.UTC())
			if inserted.Error != nil {
				return fmt.Errorf("write Outbox dead letter: %w", inserted.Error)
			}
			updated := tx.Exec(`UPDATE infra.outbox SET dead_lettered_at=?,last_error_class=?,claim_token=NULL,lease_until=NULL,update_time=?
				WHERE id=?::uuid AND create_time=? AND claim_token=?::uuid`, now.UTC(), errorClass, now.UTC(), event.ID, event.CreateTime.UTC(), event.ClaimToken)
			if updated.Error != nil {
				return fmt.Errorf("dead-letter Outbox event: %w", updated.Error)
			}
			if updated.RowsAffected != 1 {
				return ErrLeaseLost
			}
			dead = true
			return nil
		}
		updated := tx.Exec(`UPDATE infra.outbox SET next_attempt_at=?,last_error_class=?,claim_token=NULL,lease_until=NULL,update_time=?
			WHERE id=?::uuid AND create_time=? AND claim_token=?::uuid`, nextAttempt.UTC(), errorClass, now.UTC(), event.ID, event.CreateTime.UTC(), event.ClaimToken)
		if updated.Error != nil {
			return fmt.Errorf("schedule Outbox retry: %w", updated.Error)
		}
		if updated.RowsAffected != 1 {
			return ErrLeaseLost
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("record Outbox retry: %w", err)
	}
	return dead, nil
}

// ReplayDeadLetter requeues one failed event with an audited operator identity.
func (s *Store) ReplayDeadLetter(ctx context.Context, eventID uuid.UUID, createdAt time.Time, operatorID uuid.UUID, reason string, now time.Time) error {
	if eventID == uuid.Nil || operatorID == uuid.Nil || len(reason) == 0 || len(reason) > 500 {
		return fmt.Errorf("invalid Outbox replay request")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current struct{ ReplayedAt *time.Time }
		row := tx.Raw(`SELECT replayed_at FROM infra.outbox_dead_letter WHERE event_id=?::uuid AND event_create_time=? FOR UPDATE`, eventID, createdAt.UTC()).Scan(&current)
		if row.Error != nil {
			return fmt.Errorf("read Outbox dead letter: %w", row.Error)
		}
		if row.RowsAffected != 1 || current.ReplayedAt != nil {
			return fmt.Errorf("outbox dead letter is unavailable for replay")
		}
		updated := tx.Exec(`UPDATE infra.outbox SET dead_lettered_at=NULL,attempt_count=0,next_attempt_at=?,claim_token=NULL,lease_until=NULL,last_error_class=NULL,update_time=?
			WHERE id=?::uuid AND create_time=? AND dead_lettered_at IS NOT NULL AND published_at IS NULL`, now.UTC(), now.UTC(), eventID, createdAt.UTC())
		if updated.Error != nil {
			return fmt.Errorf("requeue Outbox event: %w", updated.Error)
		}
		if updated.RowsAffected != 1 {
			return ErrLeaseLost
		}
		updated = tx.Exec(`UPDATE infra.outbox_dead_letter SET replayed_at=? WHERE event_id=?::uuid AND event_create_time=? AND replayed_at IS NULL`, now.UTC(), eventID, createdAt.UTC())
		if updated.Error != nil {
			return fmt.Errorf("mark Outbox dead letter replayed: %w", updated.Error)
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("outbox dead letter changed during replay")
		}
		inserted := tx.Exec(`INSERT INTO infra.outbox_dead_letter_audit(id,event_id,event_create_time,operator_id,reason,created_at)
			VALUES (?::uuid,?::uuid,?,?,?,?)`, uuid.New(), eventID, createdAt.UTC(), operatorID, reason, now.UTC())
		if inserted.Error != nil {
			return fmt.Errorf("audit Outbox dead letter replay: %w", inserted.Error)
		}
		if inserted.RowsAffected != 1 {
			return fmt.Errorf("audit Outbox dead letter replay: inserted %d rows", inserted.RowsAffected)
		}
		return nil
	})
}

var _ application.Store = (*Store)(nil)
