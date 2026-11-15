package postgres

import (
	"context"
	"fmt"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

// HasInflightWork reads only this module's persisted work for a project.
// The caller must authorize the project and hold its row lock in the injected
// transaction, which serializes this check with admission of new work.
func (s *Store) HasInflightWork(ctx context.Context, actor mediaapp.Actor, projectID uuid.UUID) (bool, error) {
	if s == nil || s.db == nil {
		return false, ErrUnavailable
	}
	if projectID == uuid.Nil || actor.ID == uuid.Nil || actor.OrgID == uuid.Nil {
		return false, identityapp.ErrForbidden
	}
	present, err := storageExists(s.db.WithContext(ctx).Model(&assetRow{}).Where("project_id = ? AND status IN ? AND NOT is_delete", projectID, []domain.Status{domain.StatusUploading, domain.StatusProcessing}))
	if err != nil {
		return false, fmt.Errorf("read media project work: %w", err)
	}
	return present, nil
}
