package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// HasInflightProjectLifecycleWork reads retained package work for lifecycle transitions.
// The caller must authorize this exact project/scope and hold its row lock in
// the injected transaction. Library access is deliberately not used here: it
// hides recycled projects, while lifecycle restoration must inspect their work.
func (s *PackageStore) HasInflightProjectLifecycleWork(ctx context.Context, actor identityapp.Principal, project uuid.UUID) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	if project == uuid.Nil || actor.ID == uuid.Nil || actor.OrgID == uuid.Nil {
		return false, identityapp.ErrForbidden
	}
	var busy bool
	err := s.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM media.package_job WHERE org_id=? AND project_id=? AND status='needs_reconciliation')`, actor.OrgID, project).Scan(&busy).Error
	if err != nil {
		return false, fmt.Errorf("read retained package project work: %w", err)
	}
	return busy, nil
}
