package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

// HasInflightWork retains lifecycle fencing while any source write lacks a receipt.
// The caller already holds current workspace authorization and its project lock.
func (s *SourceStore) HasInflightWork(ctx context.Context, actor identityapp.Principal, project uuid.UUID) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	db := s.db.WithContext(ctx)
	var pendingCommands int64
	if err := db.Model(&scriptCommandRow{}).
		Joins("JOIN script.command_state AS command_state ON command_state.actor_id = script.command.actor_id AND command_state.request_id = script.command.request_id").
		Where("script.command.org_id = ? AND script.command.project_id = ? AND command_state.status = ?", actor.OrgID, project, scriptCommandPending).
		Count(&pendingCommands).Error; err != nil {
		return false, fmt.Errorf("read pending source commands: %w", err)
	}
	var pendingImports int64
	err := db.Model(&scriptImportJobRow{}).
		Joins("JOIN script.import_state AS import_state ON import_state.job_id = script.import_job.id").
		Where("script.import_job.org_id = ? AND script.import_job.project_id = ?", actor.OrgID, project).
		Where("(import_state.status IN ? OR import_state.needs_reconciliation = ? OR import_state.io_owner_id IS NOT NULL OR import_state.io_state = ?)", []domain.ImportStatus{domain.ImportStatusQueued, domain.ImportStatusRunning, domain.ImportStatusCancelRequested}, true, domain.ImportIOStateUnknown).
		Count(&pendingImports).Error
	if err != nil {
		return false, fmt.Errorf("read pending source imports: %w", err)
	}
	return pendingCommands != 0 || pendingImports != 0, nil
}
