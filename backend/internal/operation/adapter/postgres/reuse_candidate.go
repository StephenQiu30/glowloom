package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// FindReusableCompleted finds a completed operation with the same frozen input
// in the actor's active project. Every retained output and linked media asset
// must still be usable; confirmation must recheck this mutable state.
func (s *Store) FindReusableCompleted(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID, inputHash string) (uuid.UUID, error) {
	if s == nil || s.db == nil {
		return uuid.Nil, ErrUnavailable
	}
	if projectID == uuid.Nil || strings.TrimSpace(inputHash) == "" {
		return uuid.Nil, ErrNotFound
	}
	var candidate struct{ ID uuid.UUID }
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		result := tx.Raw(`
			SELECT o.id
			FROM operation.operation AS o
			JOIN workspace.project AS p ON p.id = o.project_id
			WHERE o.project_id = ?::uuid AND o.input_hash = ?
			  AND o.status = 'completed' AND NOT o.is_delete
			  AND p.org_id = ?::uuid AND p.status = 'active' AND NOT p.is_delete
			  AND EXISTS (
			    SELECT 1 FROM operation.operation_output AS output
			    WHERE output.project_id = o.project_id AND output.operation_id = o.id
			    GROUP BY output.operation_id
			    HAVING count(*) = o.output_count
			       AND count(*) FILTER (WHERE output.is_delete) = 0
			       AND min(output.seq_no) = 0
			       AND max(output.seq_no) = o.output_count - 1
			  )
			  AND NOT EXISTS (
			    SELECT 1
			    FROM operation.operation_output AS output
			    LEFT JOIN media.media_asset AS asset
			      ON asset.project_id = output.project_id AND asset.id = output.media_asset_id
			    WHERE output.project_id = o.project_id AND output.operation_id = o.id
			      AND (
			        output.is_delete OR output.moderation_status NOT IN ('passed', 'skipped')
			        OR (output.kind = 'media' AND (
			          asset.id IS NULL OR asset.is_delete OR asset.status <> 'ready'
			          OR asset.moderation_status NOT IN ('passed', 'skipped')
			          OR asset.contains_real_person OR asset.consent_record_id IS NOT NULL
			        ))
			      )
			  )
			ORDER BY o.create_time DESC, o.id DESC
			LIMIT 1
		`, projectID.String(), inputHash, actor.OrgID.String()).Scan(&candidate)
		if result.Error != nil {
			return fmt.Errorf("query reusable operation: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("find reusable operation: %w", err)
	}
	return candidate.ID, nil
}
