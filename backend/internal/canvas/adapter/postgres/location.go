package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// Locate reads an ownership candidate, then validates its live request authority.
func (s *Store) Locate(ctx context.Context, actor identity.Principal, id uuid.UUID) (application.Location, error) {
	var result application.Location
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var candidate struct{ ProjectID, OrgID, ScopeID uuid.UUID }
		r := tx.Raw(`SELECT p.id AS project_id,p.org_id,p.owner_scope_id AS scope_id FROM canvas.document d JOIN workspace.project p ON p.id=d.project_id WHERE d.id=? AND NOT d.is_delete AND NOT p.is_delete AND p.status IN ('active','archived')`, id).Scan(&candidate)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 || candidate.ScopeID == uuid.Nil {
			return application.ErrNotFound
		}
		// Preserve the authenticated session. Only server-owned routing fields change.
		actor.ScopeID = candidate.ScopeID
		actor.OrgID = candidate.OrgID
		if err := s.authorize(tx, actor, candidate.ProjectID, false); err != nil {
			return err
		}
		var exists bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM canvas.document WHERE id=? AND project_id=? AND NOT is_delete)`, id, candidate.ProjectID).Scan(&exists).Error; err != nil {
			return err
		}
		if !exists {
			return application.ErrNotFound
		}
		result = application.Location{CanvasID: id, ProjectID: candidate.ProjectID, ScopeID: candidate.ScopeID}
		return nil
	})
	return result, err
}
