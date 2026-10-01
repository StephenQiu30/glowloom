package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// HasInflightWork reads only this module's persisted work for a project.
// The caller must authorize the project and hold its row lock in the injected
// transaction, which serializes this check with admission of new work.
func (s *Store) HasInflightWork(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID) (bool, error) {
	if s == nil || s.db == nil {
		return false, ErrUnavailable
	}
	if projectID == uuid.Nil || actor.ID == uuid.Nil || actor.OrgID == uuid.Nil {
		return false, identityapp.ErrForbidden
	}
	var present bool
	if err := s.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM operation.operation WHERE project_id=? AND status IN ('confirmed','submitting','submitted','succeeded','ingesting','unknown','reconciling','manual','cancelling') AND NOT is_delete)`, projectID).Scan(&present).Error; err != nil {
		return false, fmt.Errorf("read operation project work: %w", err)
	}
	return present, nil
}
