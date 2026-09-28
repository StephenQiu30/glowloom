package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

// CopyReuseOutputsInTransaction locks and rechecks the completed source, then
// adds new output rows pointing at the same media assets. The caller must keep
// tx open through zero-cost settlement and the terminal Operation write.
func (s *Store) CopyReuseOutputsInTransaction(ctx context.Context, tx *gorm.DB, operationID uuid.UUID) error {
	if s == nil || s.db == nil || tx == nil {
		return ErrUnavailable
	}
	if operationID == uuid.Nil {
		return application.ErrReuseSourceUnavailable
	}
	tx = tx.WithContext(ctx)
	var row struct {
		ID           uuid.UUID
		ProjectID    uuid.UUID
		ReusedFromID *uuid.UUID
		InputHash    string
		OutputCount  int32
		Status       string
	}
	result := tx.Raw(`
		SELECT id, project_id, reused_from_id, input_hash, output_count, status
		FROM operation.operation
		WHERE id = ?::uuid AND NOT is_delete
		FOR UPDATE
	`, operationID.String()).Scan(&row)
	if result.Error != nil {
		return fmt.Errorf("lock reuse operation: %w", result.Error)
	}
	if result.RowsAffected != 1 || row.Status != "confirmed" || row.ReusedFromID == nil {
		return application.ErrReuseSourceUnavailable
	}
	var existing, calls int64
	if err := tx.Raw(`
		SELECT count(*) FROM operation.operation_output
		WHERE project_id = ?::uuid AND operation_id = ?::uuid
	`, row.ProjectID.String(), operationID.String()).Scan(&existing).Error; err != nil {
		return fmt.Errorf("check existing reused outputs: %w", err)
	}
	if err := tx.Raw(`
		SELECT count(*) FROM operation.provider_call
		WHERE project_id = ?::uuid AND operation_id = ?::uuid AND NOT is_delete
	`, row.ProjectID.String(), operationID.String()).Scan(&calls).Error; err != nil {
		return fmt.Errorf("check reuse provider calls: %w", err)
	}
	if existing != 0 || calls != 0 {
		return application.ErrReuseSourceUnavailable
	}
	outputs, available, err := lockReusableSourceOutputs(tx, reuseSourceIdentity{
		ID: row.ID, ProjectID: row.ProjectID, SourceID: *row.ReusedFromID,
		InputHash: row.InputHash, OutputCount: row.OutputCount,
	})
	if err != nil {
		return err
	}
	if !available {
		return application.ErrReuseSourceUnavailable
	}
	for _, output := range outputs {
		insert := tx.Exec(`
			INSERT INTO operation.operation_output
			  (id, project_id, operation_id, seq_no, kind, media_asset_id,
			   json_payload, moderation_status, moderation_reason)
			VALUES (?::uuid, ?::uuid, ?::uuid, ?, ?, ?::uuid,
			        ?::jsonb, ?, ?)
		`, uuid.NewString(), row.ProjectID.String(), operationID.String(),
			output.SeqNo, output.Kind, output.MediaAssetID,
			nullableJSON(output.JSONPayload), output.ModerationStatus,
			output.ModerationReason)
		if insert.Error != nil {
			return fmt.Errorf("copy reusable output %d: %w", output.SeqNo, insert.Error)
		}
		if insert.RowsAffected != 1 {
			return fmt.Errorf("copy reusable output %d: wrote %d rows", output.SeqNo, insert.RowsAffected)
		}
	}
	return nil
}
