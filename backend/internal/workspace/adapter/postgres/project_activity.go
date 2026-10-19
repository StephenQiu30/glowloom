package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

// MarkProjectContentChanged must participate in the consumer's already-authorized write transaction.
func (s *Store) MarkProjectContentChanged(ctx context.Context, actor identity.Principal, id uuid.UUID) error {
	if s == nil || s.db == nil {
		return application.ErrProjectDependencyUnavailable
	}
	if _, ok := s.db.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return application.ErrProjectDependencyUnavailable
	}
	if err := s.requireScope(ctx, s.db, actor, true); err != nil {
		return err
	}
	var present bool
	if err := s.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM workspace.project WHERE id=? AND org_id=? AND owner_scope_id=? AND NOT is_delete AND status='active')`, id, actor.OrgID, actor.ScopeID).Scan(&present).Error; err != nil {
		return err
	}
	if !present {
		return application.ErrProjectNotFound
	}
	return markProjectActivity(s.db.WithContext(ctx), id, "content")
}

func markProjectActivity(tx *gorm.DB, id uuid.UUID, basis string) error {
	result := tx.Exec(`INSERT INTO workspace.project_activity(project_id,modified_at,basis) VALUES(?,clock_timestamp(),?) ON CONFLICT(project_id) DO UPDATE SET modified_at=GREATEST(workspace.project_activity.modified_at,clock_timestamp()),basis=EXCLUDED.basis`, id, basis)
	if result.Error != nil {
		return fmt.Errorf("update project activity: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return application.ErrProjectDependencyUnavailable
	}
	return nil
}

func projectMetadataChanged(before, after domain.Project) bool {
	if before.Name != after.Name || before.Description != after.Description {
		return true
	}
	if before.CoverAssetID == nil || after.CoverAssetID == nil {
		return (before.CoverAssetID == nil) != (after.CoverAssetID == nil)
	}
	return *before.CoverAssetID != *after.CoverAssetID
}
