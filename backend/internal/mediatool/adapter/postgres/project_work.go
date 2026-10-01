package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

// HasInflightWork reads only this module's persisted work for a project.
// The caller must authorize the project and hold its row lock in the injected
// transaction, which serializes this check with admission of new work.
func (s *Store) HasInflightWork(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	if projectID == uuid.Nil || actor.ID == uuid.Nil || actor.OrgID == uuid.Nil {
		return false, identityapp.ErrForbidden
	}
	var present bool
	if err := s.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM mediatool.export_job WHERE project_id=? AND status IN ('queued','running','cancel_requested'))`, projectID).Scan(&present).Error; err != nil {
		return false, fmt.Errorf("read mediatool project work: %w", err)
	}
	return present, nil
}
