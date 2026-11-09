package postgres

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

// readModelCatalog resolves only the persisted snapshot pinned by the connection.
// A missing seed is never replaced by a separate compiled catalog.
func readModelCatalog(tx *gorm.DB, versionID uuid.UUID, document domain.PresetDocument) (domain.ModelCandidateCatalog, error) {
	var rows []struct{ Payload []byte }
	if err := tx.Raw(`SELECT payload FROM catalog.model_preset WHERE preset_version_id=? ORDER BY ordinal,model_key`, versionID).Scan(&rows).Error; err != nil {
		return domain.ModelCandidateCatalog{}, fmt.Errorf("read model preset snapshot: %w", err)
	}
	source := domain.ModelCandidateCatalog{Version: versionID.String(), Models: []domain.ModelCandidatePreset{}}
	for _, row := range rows {
		var candidate domain.ModelCandidatePreset
		if err := json.Unmarshal(row.Payload, &candidate); err != nil {
			return domain.ModelCandidateCatalog{}, fmt.Errorf("decode model preset snapshot: %w", err)
		}
		if candidate.Key == "" || candidate.AdapterKey != document.Config.AdapterKey {
			return domain.ModelCandidateCatalog{}, domain.ErrInvalidPreset
		}
		source.Models = append(source.Models, candidate)
	}
	return domain.ModelCatalogForPreset(document, source), nil
}

// copyModelPresets copies public candidates in the same version-creation transaction.
// Older versions retain their own parameters and never follow a changing head.
func copyModelPresets(tx *gorm.DB, presetID, versionID uuid.UUID, document domain.PresetDocument) error {
	var source struct{ ID uuid.UUID }
	if err := tx.Raw(`SELECT id FROM catalog.provider_preset_version WHERE preset_id=? AND id<>? ORDER BY version_no DESC LIMIT 1`, presetID, versionID).Scan(&source).Error; err != nil {
		return err
	}
	if source.ID == uuid.Nil {
		if err := tx.Raw(`SELECT v.id FROM catalog.provider_preset p JOIN catalog.provider_preset_version v ON v.preset_id=p.parent_id WHERE p.id=? ORDER BY v.version_no DESC LIMIT 1`, presetID).Scan(&source).Error; err != nil {
			return err
		}
	}
	if source.ID == uuid.Nil {
		if err := tx.Raw(`SELECT v.id FROM catalog.provider_preset p JOIN catalog.provider_preset_version v ON v.preset_id=p.id WHERE p.origin='builtin' AND v.payload->'config'->>'adapter_key'=? ORDER BY v.version_no DESC,p.id LIMIT 1`, document.Config.AdapterKey).Scan(&source).Error; err != nil {
			return err
		}
	}
	if source.ID == uuid.Nil {
		return nil
	}
	var rows []struct {
		ModelKey    string
		Ordinal     int
		Payload     []byte
		ContentHash string
	}
	if err := tx.Raw(`SELECT model_key,ordinal,payload,content_hash FROM catalog.model_preset WHERE preset_version_id=? ORDER BY ordinal`, source.ID).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		var model domain.ModelCandidatePreset
		if err := json.Unmarshal(row.Payload, &model); err != nil {
			return err
		}
		if model.AdapterKey != document.Config.AdapterKey {
			continue
		}
		if err := tx.Exec(`INSERT INTO catalog.model_preset(preset_version_id,model_key,ordinal,payload,content_hash) VALUES(?,?,?,?::jsonb,?)`, versionID, row.ModelKey, row.Ordinal, string(row.Payload), row.ContentHash).Error; err != nil {
			return err
		}
	}
	return nil
}

// validatePinnedModelVersion checks the source snapshot, not an editable form field.
func validatePinnedModelVersion(tx *gorm.DB, version domain.ModelVersion) error {
	var row struct{ CandidateSnapshot []byte }
	if err := tx.Raw(`SELECT candidate_snapshot FROM catalog.model_profile WHERE id=? AND NOT is_delete`, version.ModelID).Scan(&row).Error; err != nil {
		return err
	}
	if len(row.CandidateSnapshot) == 0 {
		return nil
	}
	var source domain.ModelCandidateSelection
	if err := json.Unmarshal(row.CandidateSnapshot, &source); err != nil {
		return domain.ErrInvalidModelVersion
	}
	return source.Model.ValidatePinnedVersion(version)
}
