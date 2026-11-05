package postgres

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
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
	var count int64
	if err := s.db.WithContext(ctx).Model(&jobRow{}).
		Where("project_id = ? AND org_id = ? AND status IN ?", projectID, actor.OrgID, []domain.Status{domain.Queued, domain.Running, domain.CancelRequested}).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("read mediatool project work: %w", err)
	}
	return count > 0, nil
}
