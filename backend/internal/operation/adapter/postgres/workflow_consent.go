package postgres

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

var (
	// ErrWorkflowInputNotReady means frozen media is missing, unready, or unapproved.
	ErrWorkflowInputNotReady = application.ErrWorkflowInputNotReady
	// ErrWorkflowConsentUnavailable fails closed until consent records are queryable.
	ErrWorkflowConsentUnavailable = application.ErrWorkflowConsentUnavailable
)

// CheckSubmissionInputs verifies frozen media before a provider request. The
// submitting transition repeats this check under the same transaction as its
// state write, so a later media change cannot slip between check and submit.
func (s *Store) CheckSubmissionInputs(ctx context.Context, operationID uuid.UUID) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if operationID == uuid.Nil {
		return ErrNotFound
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var operation struct{ ProjectID uuid.UUID }
		result := tx.Raw(`
			SELECT project_id FROM operation.operation
			WHERE id = ?::uuid AND NOT is_delete FOR SHARE
		`, operationID.String()).Scan(&operation)
		if result.Error != nil {
			return fmt.Errorf("read submission operation: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		return checkWorkflowInputsForSubmission(tx, operationID, operation.ProjectID)
	})
	if err != nil {
		return fmt.Errorf("check workflow submission inputs: %w", err)
	}
	return nil
}

func checkWorkflowInputsForSubmission(tx *gorm.DB, operationID, projectID uuid.UUID) error {
	var inputs []struct {
		MediaAssetID *uuid.UUID
		MaskAssetID  *uuid.UUID
	}
	result := tx.Raw(`
		SELECT media_asset_id, mask_asset_id
		FROM operation.operation_input
		WHERE operation_id = ?::uuid AND NOT is_delete
		ORDER BY seq_no
	`, operationID.String()).Scan(&inputs)
	if result.Error != nil {
		return fmt.Errorf("read frozen submission media: %w", result.Error)
	}
	assetIDs := make(map[uuid.UUID]struct{})
	for _, input := range inputs {
		for _, id := range []*uuid.UUID{input.MediaAssetID, input.MaskAssetID} {
			if id != nil {
				assetIDs[*id] = struct{}{}
			}
		}
	}
	ordered := make([]uuid.UUID, 0, len(assetIDs))
	for id := range assetIDs {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].String() < ordered[j].String() })
	for _, id := range ordered {
		var asset struct {
			Status             string
			ModerationStatus   string
			ContainsRealPerson bool
			ConsentRecordID    *uuid.UUID
		}
		result = tx.Raw(`
			SELECT status, moderation_status, contains_real_person, consent_record_id
			FROM media.media_asset
			WHERE id = ?::uuid AND project_id = ?::uuid AND NOT is_delete
			FOR SHARE
		`, id.String(), projectID.String()).Scan(&asset)
		if result.Error != nil {
			return fmt.Errorf("lock frozen submission asset: %w", result.Error)
		}
		if result.RowsAffected != 1 || asset.Status != "ready" || asset.ModerationStatus != "passed" {
			return ErrWorkflowInputNotReady
		}
		// media.consent_record is not migrated yet. A real-person asset or
		// recorded consent must never be assumed valid from the asset alone.
		if asset.ContainsRealPerson || asset.ConsentRecordID != nil {
			return ErrWorkflowConsentUnavailable
		}
	}
	return nil
}
