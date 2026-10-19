package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// ReadProjectNavigation uses the caller's authorized transaction and never guesses a replacement main document.
func (s *Store) ReadProjectNavigation(ctx context.Context, actor identity.Principal, id uuid.UUID) (application.ProjectNavigation, error) {
	var result application.ProjectNavigation
	if s == nil || s.db == nil {
		return result, ErrUnavailable
	}
	if err := s.requireScope(ctx, s.db, actor, false); err != nil {
		return result, err
	}
	read := s.db.WithContext(ctx).Raw(`SELECT id AS project_id,initial_canvas_id AS main_canvas_id,name,status,is_delete FROM workspace.project WHERE id=? AND org_id=? AND owner_scope_id=? AND NOT is_delete FOR SHARE`, id, actor.OrgID, actor.ScopeID).Scan(&result)
	if read.Error != nil {
		return result, fmt.Errorf("read project navigation: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return result, application.ErrProjectNotFound
	}
	return result, nil
}
