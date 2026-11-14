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

// validateProviderLogo only permits the dedicated normalized upload, never avatars.
func validateProviderLogo(tx *gorm.DB, actor identityapp.Principal, id *uuid.UUID) error {
	if id == nil {
		return nil
	}
	var eligible struct{ ID uuid.UUID }
	result := tx.Raw(`SELECT id FROM media.media_asset WHERE id=? AND personal_actor_id=? AND personal_org_id=? AND project_id IS NULL
 AND kind='image' AND origin='upload' AND status='ready' AND moderation_status='passed' AND NOT is_delete
 AND mime_type='image/png' AND codec='png' AND byte_size BETWEEN 1 AND 6291456
 AND width BETWEEN 1 AND 1024 AND height BETWEEN 1 AND 1024
 AND moderation_detail->'provider_logo'->>'method'='server_reencode_png'
 AND moderation_detail->'provider_logo'->>'version'='1' FOR SHARE`, id, actor.ID, actor.OrgID).Scan(&eligible)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrInvalidPreset
	}
	return nil
}

// CreateCustomProvider atomically saves an empty editing snapshot and receipt.
func (s *Store) CreateCustomProvider(ctx context.Context, actor identityapp.Principal, input application.CustomProviderInput) (application.PresetConnection, error) {
	return catalogMutation(ctx, s.db, s.actors, actor, "provider.custom.connect", uuid.Nil, input.RequestID, input, func(tx *gorm.DB) (application.PresetConnection, error) {
		if err := validateProviderLogo(tx, actor, input.LogoAssetID); err != nil {
			return application.PresetConnection{}, err
		}
		providerID, configID := uuid.New(), uuid.New()
		config := input.Config
		if err := tx.Exec(`INSERT INTO catalog.provider(id,key,name,adapter_key,region,status,concurrency_limit,rate_limit_per_min,logo_asset_id) VALUES(?,?,?,?,?,'disabled',?,?,?)`, providerID, "custom-"+providerID.String(), input.Name, config.AdapterKey, config.Region, config.ConcurrencyLimit, config.RateLimitPerMin, input.LogoAssetID).Error; err != nil {
			return application.PresetConnection{}, err
		}
		payload, err := json.Marshal(config)
		if err != nil {
			return application.PresetConnection{}, err
		}
		hash := sha256.Sum256(payload)
		if err := tx.Exec(`INSERT INTO catalog.provider_config_version(id,provider_id,preset_version_id,version_no,config,content_hash) VALUES(?,?,NULL,1,?::jsonb,?)`, configID, providerID, string(payload), hex.EncodeToString(hash[:])).Error; err != nil {
			return application.PresetConnection{}, err
		}
		if err := tx.Exec(`UPDATE catalog.provider SET current_config_version_id=? WHERE id=?`, configID, providerID).Error; err != nil {
			return application.PresetConnection{}, err
		}
		catalog := domain.ModelCandidateCatalog{Version: configID.String(), Models: []domain.ModelCandidatePreset{}}
		return application.PresetConnection{ProviderID: providerID, Config: application.ConnectionConfig{ID: configID, ProviderID: providerID, Version: 1, Config: config, ModelCatalog: &catalog}}, nil
	})
}

// SetProviderLogo changes only display metadata and retains immutable execution snapshots.
func (s *Store) SetProviderLogo(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input application.ProviderLogoInput) (application.CreatedProvider, error) {
	return catalogMutation(ctx, s.db, s.actors, actor, "provider.logo.set", id, input.RequestID, input, func(tx *gorm.DB) (application.CreatedProvider, error) {
		var current providerRow
		result := tx.Raw(`SELECT * FROM catalog.provider WHERE id=? AND NOT is_delete FOR UPDATE`, id).Scan(&current)
		if result.Error != nil {
			return application.CreatedProvider{}, result.Error
		}
		if result.RowsAffected != 1 {
			return application.CreatedProvider{}, application.ErrProviderNotFound
		}
		if current.Revision != input.ExpectedRevision {
			return application.CreatedProvider{}, domain.ErrProviderRevisionConflict
		}
		if err := validateProviderLogo(tx, actor, input.LogoAssetID); err != nil {
			return application.CreatedProvider{}, err
		}
		result = tx.Exec(`UPDATE catalog.provider SET logo_asset_id=?,revision=revision+1,update_time=now() WHERE id=?`, input.LogoAssetID, id)
		if result.Error != nil {
			return application.CreatedProvider{}, result.Error
		}
		var saved providerListRow
		if err := tx.Raw(`SELECT id AS provider_id,key,name,adapter_key,region,status AS provider_status,concurrency_limit,rate_limit_per_min,revision,create_time AS provider_create_time,update_time AS provider_update_time,logo_asset_id FROM catalog.provider WHERE id=?`, id).Scan(&saved).Error; err != nil {
			return application.CreatedProvider{}, err
		}
		item, err := saved.item()
		return item.Provider, err
	})
}
