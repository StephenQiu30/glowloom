package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ErrInvalidPruneBatchSize means a cleanup batch cannot make progress.
var ErrInvalidPruneBatchSize = errors.New("prune batch size must be positive")

const publishedEventRetention = 7 * 24 * time.Hour

// PartitionStore creates Outbox month partitions and moves overflow rows.
type PartitionStore struct {
	db *gorm.DB
}

// NewPartitionStore constructs a PostgreSQL partition maintainer.
func NewPartitionStore(db *gorm.DB) *PartitionStore {
	return &PartitionStore{db: db}
}

// EnsurePartitions makes the current UTC month and next three months writable.
// One transaction blocks Outbox writes while it moves default rows and attaches
// missing partitions; failures roll back both the move and the attachment.
func (s *PartitionStore) EnsurePartitions(ctx context.Context, now time.Time) error {
	current := now.UTC()
	first := time.Date(current.Year(), current.Month(), 1, 0, 0, 0, 0, time.UTC)
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("LOCK TABLE infra.outbox IN ACCESS EXCLUSIVE MODE").Error; err != nil {
			return fmt.Errorf("lock outbox for partition maintenance: %w", err)
		}
		for offset := range 4 {
			month := first.AddDate(0, offset, 0)
			if err := ensureMonth(tx, month); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("ensure outbox partitions: %w", err)
	}
	return nil
}

// PrunePublished deletes one batch of events published more than seven days ago.
// Pending and locked events are skipped; run it periodically to revisit them.
func (s *PartitionStore) PrunePublished(ctx context.Context, now time.Time, batchSize int) (int64, error) {
	if batchSize <= 0 {
		return 0, ErrInvalidPruneBatchSize
	}
	result := s.db.WithContext(ctx).Exec(`
		WITH expired AS (
			SELECT id, create_time FROM infra.outbox
			WHERE published_at < ?
			ORDER BY published_at, create_time, id
			LIMIT ?
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM infra.outbox AS event
		USING expired
		WHERE event.id = expired.id AND event.create_time = expired.create_time
	`, now.UTC().Add(-publishedEventRetention), batchSize)
	if result.Error != nil {
		return 0, fmt.Errorf("prune published outbox events: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func ensureMonth(tx *gorm.DB, month time.Time) error {
	name := "outbox_" + month.Format("200601")
	end := month.AddDate(0, 1, 0)
	var attached bool
	if err := tx.Raw(`
		SELECT EXISTS (
			SELECT 1 FROM pg_inherits AS i
			JOIN pg_class AS c ON c.oid = i.inhrelid
			JOIN pg_namespace AS n ON n.oid = c.relnamespace
			WHERE i.inhparent = 'infra.outbox'::regclass
			  AND n.nspname = 'infra' AND c.relname = ?
		)
	`, name).Scan(&attached).Error; err != nil {
		return fmt.Errorf("check outbox partition %s: %w", name, err)
	}
	if attached {
		return nil
	}
	// name is derived solely from a UTC month, never from user-supplied SQL.
	if err := tx.Exec("CREATE TABLE infra." + name + " (LIKE infra.outbox INCLUDING ALL)").Error; err != nil {
		return fmt.Errorf("create outbox partition %s: %w", name, err)
	}
	move := `
		WITH moved AS (
			DELETE FROM infra.outbox_default
			WHERE create_time >= ? AND create_time < ?
			RETURNING id, topic, partition_key, payload, headers,
			          create_time, published_at, update_time, is_delete
		)
		INSERT INTO infra.` + name + ` (
			id, topic, partition_key, payload, headers,
			create_time, published_at, update_time, is_delete
		)
		SELECT id, topic, partition_key, payload, headers,
		       create_time, published_at, update_time, is_delete FROM moved
	`
	if err := tx.Exec(move, month, end).Error; err != nil {
		return fmt.Errorf("move overflow events into %s: %w", name, err)
	}
	attach := fmt.Sprintf(
		"ALTER TABLE infra.outbox ATTACH PARTITION infra.%s FOR VALUES FROM ('%s') TO ('%s')",
		name, month.Format(time.RFC3339), end.Format(time.RFC3339),
	)
	if err := tx.Exec(attach).Error; err != nil {
		return fmt.Errorf("attach outbox partition %s: %w", name, err)
	}
	return nil
}
