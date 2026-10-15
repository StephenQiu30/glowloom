package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func readConnectionConfig(tx *gorm.DB, id uuid.UUID) (application.ConnectionConfig, int64, error) {
	var row struct {
		ID, ProviderID, PresetVersionID uuid.UUID
		Version                         int
		Revision                        int64
		Config                          []byte
	}
	r := tx.Raw(`SELECT c.id,c.provider_id,c.preset_version_id,c.version_no AS version,c.config,p.revision FROM catalog.provider p JOIN catalog.provider_config_version c ON c.id=p.current_config_version_id WHERE p.id=? AND NOT p.is_delete`, id).Scan(&row)
	if r.Error != nil {
		return application.ConnectionConfig{}, 0, r.Error
	}
	if r.RowsAffected != 1 {
		return application.ConnectionConfig{}, 0, application.ErrProviderNotFound
	}
	var config domain.ProviderConfig
	if err := json.Unmarshal(row.Config, &config); err != nil {
		return application.ConnectionConfig{}, 0, err
	}
	return application.ConnectionConfig{ID: row.ID, ProviderID: row.ProviderID, PresetVersionID: row.PresetVersionID, Version: row.Version, Config: config}, row.Revision, nil
}

// ReadConnectionConfig rechecks the live administrator before exposing settings.
func (s *Store) ReadConnectionConfig(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (application.ConnectionConfig, error) {
	var config application.ConnectionConfig
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requirePlatformCatalogAdmin(tx, actor); err != nil {
			return err
		}
		var err error
		config, _, err = readConnectionConfig(tx, id)
		return err
	})
	return config, err
}

func previewConfigUpgrade(tx *gorm.DB, id uuid.UUID, input application.ConfigUpgradeInput) (application.ConfigUpgradePreview, error) {
	current, revision, err := readConnectionConfig(tx, id)
	if err != nil {
		return application.ConfigUpgradePreview{}, err
	}
	if current.ID != input.ExpectedConfigID || revision != input.ExpectedRevision {
		return application.ConfigUpgradePreview{}, application.ErrPresetConflict
	}
	var row struct{ Old, Next []byte }
	r := tx.Raw(`SELECT old.payload AS old,next.payload AS next FROM catalog.provider_preset_version old JOIN catalog.provider_preset_version next ON next.preset_id=old.preset_id JOIN catalog.provider_preset p ON p.id=next.preset_id WHERE old.id=? AND next.id=? AND NOT p.archived AND NOT p.emergency_disabled FOR SHARE OF p`, current.PresetVersionID, input.PresetVersionID).Scan(&row)
	if r.Error != nil {
		return application.ConfigUpgradePreview{}, r.Error
	}
	if r.RowsAffected != 1 {
		return application.ConfigUpgradePreview{}, application.ErrPresetUnavailable
	}
	var old, next domain.PresetDocument
	if err := json.Unmarshal(row.Old, &old); err != nil {
		return application.ConfigUpgradePreview{}, err
	}
	if err := json.Unmarshal(row.Next, &next); err != nil {
		return application.ConfigUpgradePreview{}, err
	}
	merged, conflicts := domain.MergePreset(old.Config, next.Config, current.Config)
	preview := application.ConfigUpgradePreview{Current: current, Proposed: merged, Conflicts: conflicts, Revision: revision}
	preview.Hash = application.ConfigPreviewHash(preview, input.PresetVersionID)
	return preview, nil
}

// PreviewConfigUpgrade calculates a conflict report without changing execution.
func (s *Store) PreviewConfigUpgrade(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input application.ConfigUpgradeInput) (application.ConfigUpgradePreview, error) {
	var preview application.ConfigUpgradePreview
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requirePlatformCatalogAdmin(tx, actor); err != nil {
			return err
		}
		var err error
		preview, err = previewConfigUpgrade(tx, id, input)
		return err
	})
	return preview, err
}

// ApplyConfigUpgrade preserves old snapshots, credentials and release bindings.
func (s *Store) ApplyConfigUpgrade(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input application.ApplyConfigInput) (application.ConnectionConfig, error) {
	return catalogMutation(ctx, s.db, actor, "config.upgrade", id, input.RequestID, input, func(tx *gorm.DB) (application.ConnectionConfig, error) {
		// Lock the preset first, then provider, consistently with dispatch admission.
		var locked int
		if err := tx.Raw(`SELECT 1 FROM catalog.provider_preset p JOIN catalog.provider_preset_version v ON v.preset_id=p.id WHERE v.id=? FOR SHARE OF p`, input.Upgrade.PresetVersionID).Scan(&locked).Error; err != nil {
			return application.ConnectionConfig{}, err
		}
		if err := tx.Raw(`SELECT 1 FROM catalog.provider WHERE id=? FOR UPDATE`, id).Scan(&locked).Error; err != nil {
			return application.ConnectionConfig{}, err
		}
		preview, err := previewConfigUpgrade(tx, id, input.Upgrade)
		if err != nil {
			return application.ConnectionConfig{}, err
		}
		if preview.Hash != input.PreviewHash {
			return application.ConnectionConfig{}, application.ErrPresetConflict
		}
		// First release permits only the original adapter and official endpoint.
		if input.Resolution.AdapterKey != preview.Current.Config.AdapterKey || input.Resolution.BaseURL != preview.Current.Config.BaseURL {
			return application.ConnectionConfig{}, domain.ErrInvalidPreset
		}
		data, err := json.Marshal(input.Resolution)
		if err != nil {
			return application.ConnectionConfig{}, err
		}
		sum := sha256.Sum256(data)
		result := application.ConnectionConfig{ID: uuid.New(), ProviderID: id, PresetVersionID: input.Upgrade.PresetVersionID, Version: preview.Current.Version + 1, Config: input.Resolution}
		if err := tx.Exec(`INSERT INTO catalog.provider_config_version(id,provider_id,preset_version_id,version_no,config,content_hash) VALUES(?,?,?,?,?::jsonb,?)`, result.ID, id, result.PresetVersionID, result.Version, string(data), hex.EncodeToString(sum[:])).Error; err != nil {
			return application.ConnectionConfig{}, err
		}
		if err := tx.Exec(`UPDATE catalog.provider SET current_config_version_id=?,revision=revision+1,update_time=now() WHERE id=?`, result.ID, id).Error; err != nil {
			return application.ConnectionConfig{}, err
		}
		return result, nil
	})
}
