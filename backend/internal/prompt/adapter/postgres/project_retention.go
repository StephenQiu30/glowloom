package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProjectRetentionStore clears private drafts in an already-authorized retention transaction.
type ProjectRetentionStore struct{ tx *gorm.DB }

// NewProjectRetentionStore binds the caller's project deletion batch lock.
func NewProjectRetentionStore(tx *gorm.DB) *ProjectRetentionStore {
	return &ProjectRetentionStore{tx: tx}
}

// ClearProjectContent discards draft payloads but retains idempotency metadata.
func (s *ProjectRetentionStore) ClearProjectContent(ctx context.Context, project uuid.UUID) error {
	if s == nil || s.tx == nil || project == uuid.Nil {
		return fmt.Errorf("invalid prompt retention binding")
	}
	if err := s.tx.WithContext(ctx).Exec(`DELETE FROM workspace.prompt_draft WHERE project_id=?`, project).Error; err != nil {
		return fmt.Errorf("clear expired prompt draft: %w", err)
	}
	if err := s.tx.WithContext(ctx).Exec(`UPDATE workspace.prompt_draft_receipt SET response='{"retention_expired":true}'::jsonb WHERE project_id=?`, project).Error; err != nil {
		return fmt.Errorf("clear expired prompt receipt payload: %w", err)
	}
	return nil
}
