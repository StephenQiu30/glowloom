package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

// HasInflightWork includes unknown native inference that has no cessation receipt.
// The owning project caller must authorize and retain its project row lock in the
// injected transaction, which also serializes admission of transcription jobs.
func (s *TranscriptionStore) HasInflightWork(ctx context.Context, actor identityapp.Principal, project uuid.UUID) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	if project == uuid.Nil || actor.ID == uuid.Nil || actor.OrgID == uuid.Nil {
		return false, identityapp.ErrForbidden
	}
	var present bool
	if err := s.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM mediatool.transcription_job WHERE project_id=? AND org_id=? AND status IN ('queued','running','cancel_requested'))`, project, actor.OrgID).Scan(&present).Error; err != nil {
		return false, fmt.Errorf("read transcription project work: %w", err)
	}
	return present, nil
}
