package postgres

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
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
	var count int64
	if err := s.db.WithContext(ctx).Model(&transcriptionRow{}).
		Where("project_id = ? AND org_id = ? AND status IN ?", project, actor.OrgID,
			[]domain.TranscriptionStatus{domain.TranscriptionQueued, domain.TranscriptionRunning, domain.TranscriptionCancelRequested}).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("read transcription project work: %w", err)
	}
	return count > 0, nil
}
