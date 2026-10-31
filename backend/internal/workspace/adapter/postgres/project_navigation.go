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
	read := s.db.WithContext(ctx).Raw(`SELECT p.id AS project_id,p.initial_canvas_id AS main_canvas_id,p.name,p.status,p.is_delete,COALESCE(u.display_name,'') AS owner_name FROM workspace.project p LEFT JOIN identity."user" u ON u.id=p.owner_id AND NOT u.is_delete WHERE p.id=? AND p.org_id=? AND p.owner_scope_id=? AND NOT p.is_delete FOR SHARE OF p`, id, actor.OrgID, actor.ScopeID).Scan(&result)
	if read.Error != nil {
		return result, fmt.Errorf("read project navigation: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return result, application.ErrProjectNotFound
	}
	return result, nil
}
