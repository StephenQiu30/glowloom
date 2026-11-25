package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// RequireOwnedProject rejects missing and other creators' project scopes alike.
func (s *Store) RequireOwnedProject(ctx context.Context, actor identityapp.Principal, id uuid.UUID) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorizeProject(ctx, tx, actor, id, false); err != nil {
			return err
		}
		var exists int
		result := tx.Raw(`SELECT 1 FROM workspace.project WHERE id=? AND org_id=? AND owner_scope_id=? FOR SHARE`, id, actor.OrgID, actor.ScopeID).Scan(&exists)
		if result.Error != nil {
			return fmt.Errorf("check project owner: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrProjectNotFound
		}
		return nil
	})
}
