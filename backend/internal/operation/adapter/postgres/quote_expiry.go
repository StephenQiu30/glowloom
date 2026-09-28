package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

// ExpireQuoted advances only due quotes and batches whose live items all
// expired. The second transaction lets a later run recover a batch if a
// process stops after expiring its final item.
func (s *Store) ExpireQuoted(ctx context.Context, now time.Time, batchSize int) (application.ExpiredQuotes, error) {
	if s == nil || s.db == nil {
		return application.ExpiredQuotes{}, ErrUnavailable
	}
	if now.IsZero() || batchSize < 1 || batchSize > 1000 {
		return application.ExpiredQuotes{}, application.ErrInvalidQuoteExpiry
	}
	var counts application.ExpiredQuotes
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var result struct{ Operations int64 }
		if err := tx.Raw(`
			WITH due AS (
			  SELECT id FROM operation.operation
			  WHERE status = 'quoted' AND quote_expires_at <= ? AND NOT is_delete
			  ORDER BY quote_expires_at, id LIMIT ? FOR UPDATE SKIP LOCKED
			), expired AS (
			  UPDATE operation.operation AS o
			  SET status = 'expired', failure_code = 'quote_expired', update_time = ?
			  FROM due WHERE o.id = due.id AND o.status = 'quoted'
			  RETURNING o.id
			)
			SELECT count(*) AS operations FROM expired
		`, now.UTC(), batchSize, now.UTC()).Scan(&result).Error; err != nil {
			return fmt.Errorf("expire due operation quotes: %w", err)
		}
		counts.Operations = result.Operations
		return nil
	})
	if err != nil {
		return counts, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []struct{ ID uuid.UUID }
		if err := tx.Raw(`
			SELECT b.id FROM operation.batch AS b
			WHERE b.status = 'quoted' AND NOT b.is_delete
			  AND EXISTS (
			    SELECT 1 FROM operation.operation AS o
			    WHERE o.batch_id = b.id AND NOT o.is_delete
			  )
			  AND NOT EXISTS (
			    SELECT 1 FROM operation.operation AS o
			    WHERE o.batch_id = b.id AND o.status <> 'expired' AND NOT o.is_delete
			  )
			ORDER BY b.create_time, b.id LIMIT ? FOR UPDATE OF b SKIP LOCKED
		`, batchSize).Scan(&rows).Error; err != nil {
			return fmt.Errorf("find fully expired batches: %w", err)
		}
		for _, row := range rows {
			updated := tx.Exec(`
				UPDATE operation.batch AS b
				SET status = 'expired', update_time = ?
				WHERE b.id = ?::uuid AND b.status = 'quoted' AND NOT b.is_delete
				  AND NOT EXISTS (
				    SELECT 1 FROM operation.operation AS o
				    WHERE o.batch_id = b.id AND o.status <> 'expired' AND NOT o.is_delete
				  )
			`, now.UTC(), row.ID.String())
			if updated.Error != nil {
				return fmt.Errorf("expire batch %s: %w", row.ID, updated.Error)
			}
			counts.Batches += updated.RowsAffected
		}
		return nil
	})
	if err != nil {
		return counts, err
	}
	return counts, nil
}
