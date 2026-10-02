package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
)

// HasInflightWork retains lifecycle fencing while any source write lacks a receipt.
// The caller already holds current workspace authorization and its project lock.
func (s *SourceStore) HasInflightWork(ctx context.Context, actor identityapp.Principal, project uuid.UUID) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	var found bool
	err := s.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM script.command c JOIN script.command_state s ON s.actor_id=c.actor_id AND s.request_id=c.request_id WHERE c.org_id=? AND c.project_id=? AND s.status='pending') OR EXISTS(SELECT 1 FROM script.import_job j JOIN script.import_state s ON s.job_id=j.id WHERE j.org_id=? AND j.project_id=? AND (s.status IN ('queued','running','cancel_requested') OR s.needs_reconciliation OR s.io_owner_id IS NOT NULL OR s.io_state='unknown'))`, actor.OrgID, project, actor.OrgID, project).Scan(&found).Error
	if err != nil {
		return false, fmt.Errorf("read source write ownership fence: %w", err)
	}
	return found, nil
}
