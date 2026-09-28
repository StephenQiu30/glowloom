package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

type reuseSourceIdentity struct {
	ID          uuid.UUID
	ProjectID   uuid.UUID
	SourceID    uuid.UUID
	InputHash   string
	OutputCount int32
}

type reusableOutput struct {
	ID               uuid.UUID
	SeqNo            int32
	Kind             string
	MediaAssetID     *uuid.UUID
	JSONPayload      []byte
	ModerationStatus string
	ModerationReason *string
	IsDelete         bool
}

// ReadReuseSourceAvailable checks the frozen reuse source, every output, and
// linked media under row locks. Confirmation must call it inside its own write
// transaction so those locks remain held through reservation and status write.
func (s *Store) ReadReuseSourceAvailable(ctx context.Context, actor identityapp.Principal, quoted domain.Operation) (bool, error) {
	if s == nil || s.db == nil {
		return false, ErrUnavailable
	}
	if err := quoted.Validate(); err != nil {
		return false, err
	}
	if quoted.Status != domain.StatusQuoted || quoted.Origin == "upload" {
		return false, domain.ErrQuoteNotConfirmable
	}
	var available bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		var project int
		result := tx.Raw(`
			SELECT 1 FROM workspace.project
			WHERE id = ?::uuid AND org_id = ?::uuid
			  AND status = 'active' AND NOT is_delete
			FOR SHARE
		`, quoted.ProjectID.String(), actor.OrgID.String()).Scan(&project)
		if result.Error != nil {
			return fmt.Errorf("lock reuse project: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		if quoted.ReusedFromID == nil {
			available = true
			return nil
		}
		var err error
		_, available, err = lockReusableSourceOutputs(tx, reuseSourceIdentity{
			ID: quoted.ID, ProjectID: quoted.ProjectID,
			SourceID: *quoted.ReusedFromID, InputHash: quoted.InputHash,
			OutputCount: quoted.OutputCount,
		})
		return err
	})
	if err != nil {
		return false, fmt.Errorf("read reuse source availability: %w", err)
	}
	return available, nil
}

func reusableModeration(status string) bool { return status == "passed" || status == "skipped" }

// lockReusableSourceOutputs holds the source, every output, and linked media
// until the caller's transaction ends. A source row UPDATE lock also blocks
// concurrent output inserts through its foreign key.
func lockReusableSourceOutputs(tx *gorm.DB, identity reuseSourceIdentity) ([]reusableOutput, bool, error) {
	if identity.ID == uuid.Nil || identity.ProjectID == uuid.Nil ||
		identity.SourceID == uuid.Nil || identity.SourceID == identity.ID ||
		identity.InputHash == "" || identity.OutputCount < 1 {
		return nil, false, nil
	}
	var source struct{ OutputCount int32 }
	result := tx.Raw(`
		SELECT output_count FROM operation.operation
		WHERE id = ?::uuid AND project_id = ?::uuid AND input_hash = ?
		  AND status = 'completed' AND NOT is_delete
		FOR UPDATE
	`, identity.SourceID.String(), identity.ProjectID.String(), identity.InputHash).Scan(&source)
	if result.Error != nil {
		return nil, false, fmt.Errorf("lock reuse source: %w", result.Error)
	}
	if result.RowsAffected != 1 || source.OutputCount != identity.OutputCount {
		return nil, false, nil
	}
	var outputs []reusableOutput
	result = tx.Raw(`
		SELECT id, seq_no, kind, media_asset_id, json_payload,
		       moderation_status, moderation_reason, is_delete
		FROM operation.operation_output
		WHERE project_id = ?::uuid AND operation_id = ?::uuid
		ORDER BY seq_no
		FOR SHARE
	`, identity.ProjectID.String(), identity.SourceID.String()).Scan(&outputs)
	if result.Error != nil {
		return nil, false, fmt.Errorf("lock reuse outputs: %w", result.Error)
	}
	if len(outputs) != int(identity.OutputCount) {
		return nil, false, nil
	}
	for index, output := range outputs {
		if output.IsDelete || output.SeqNo != int32(index) ||
			!reusableModeration(output.ModerationStatus) {
			return nil, false, nil
		}
		switch output.Kind {
		case "json":
			if len(output.JSONPayload) == 0 || output.MediaAssetID != nil {
				return nil, false, nil
			}
		case "media":
			if output.MediaAssetID == nil || len(output.JSONPayload) != 0 {
				return nil, false, nil
			}
			var asset struct {
				Status             string
				ModerationStatus   string
				ContainsRealPerson bool
				ConsentRecordID    *uuid.UUID
				IsDelete           bool
			}
			result = tx.Raw(`
				SELECT status, moderation_status, contains_real_person,
				       consent_record_id, is_delete
				FROM media.media_asset
				WHERE id = ?::uuid AND project_id = ?::uuid
				FOR SHARE
			`, output.MediaAssetID.String(), identity.ProjectID.String()).Scan(&asset)
			if result.Error != nil {
				return nil, false, fmt.Errorf("lock reuse media asset: %w", result.Error)
			}
			if result.RowsAffected != 1 || asset.IsDelete || asset.Status != "ready" ||
				!reusableModeration(asset.ModerationStatus) || asset.ContainsRealPerson ||
				asset.ConsentRecordID != nil {
				return nil, false, nil
			}
		default:
			return nil, false, nil
		}
	}
	return outputs, true, nil
}
