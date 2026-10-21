package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProjectRetentionStore clears graph payloads inside a workspace-owned expired
// project transaction. Document identities and command/receipt metadata remain.
type ProjectRetentionStore struct{ tx *gorm.DB }

// NewProjectRetentionStore binds the caller's locked retention transaction.
func NewProjectRetentionStore(tx *gorm.DB) *ProjectRetentionStore {
	return &ProjectRetentionStore{tx: tx}
}

// ClearProjectContent removes private graph payloads, never media entities/blobs.
func (s *ProjectRetentionStore) ClearProjectContent(ctx context.Context, project uuid.UUID) error {
	if s == nil || s.tx == nil || project == uuid.Nil {
		return fmt.Errorf("invalid canvas retention binding")
	}
	statements := []string{
		`DELETE FROM canvas.edge WHERE document_id IN (SELECT id FROM canvas.document WHERE project_id=?)`,
		`DELETE FROM canvas.node WHERE document_id IN (SELECT id FROM canvas.document WHERE project_id=?)`,
		`UPDATE canvas.command_log SET commands='[]'::jsonb,is_delete=true WHERE document_id IN (SELECT id FROM canvas.document WHERE project_id=?)`,
		`UPDATE canvas.save_receipt SET response_body='{"retention_expired":true}'::jsonb WHERE document_id IN (SELECT id FROM canvas.document WHERE project_id=?)`,
		`UPDATE canvas.document SET is_delete=true,name='已删除画布',scope='{}'::jsonb,viewport='{}'::jsonb WHERE project_id=?`,
	}
	for _, statement := range statements {
		if err := s.tx.WithContext(ctx).Exec(statement, project).Error; err != nil {
			return fmt.Errorf("clear expired canvas payload: %w", err)
		}
	}
	return nil
}
