package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

// EnsureIndependentCreation resolves a hidden, stable media container after
// rechecking the active creation scope in the same transaction.
func (s *Store) EnsureIndependentCreation(ctx context.Context, actor identity.Principal) (domain.Project, error) {
	if s == nil || s.db == nil {
		return domain.Project{}, ErrUnavailable
	}
	var project domain.Project
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireScope(ctx, tx, actor, true); err != nil {
			return err
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "independent-creation/"+actor.ScopeID.String()).Error; err != nil {
			return fmt.Errorf("lock independent creation: %w", err)
		}
		var row struct{ ID uuid.UUID }
		read := tx.Raw(`SELECT id FROM workspace.project
            WHERE owner_scope_id=? AND org_id=? AND independent_media AND status='active' AND NOT is_delete`, actor.ScopeID, actor.OrgID).Scan(&row)
		if read.Error != nil {
			return fmt.Errorf("read independent creation: %w", read.Error)
		}
		if read.RowsAffected == 0 {
			row.ID = uuid.New()
			defaults := domain.Project{
				ID: row.ID, OrgID: actor.OrgID, OwnerID: actor.ID,
				IndependentMedia: true, Name: "独立创作",
				AspectRatio: "16:9", StyleType: "realistic",
				Resolution: "1080p", Status: domain.ProjectStatusActive, Revision: 1,
			}
			if err := createProjectRowsInTx(tx, defaults, actor.ScopeID); err != nil {
				return err
			}
		}
		var err error
		project, err = readProjectInTx(tx, actor.OrgID, row.ID)
		return err
	})
	return project, err
}
