package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

type releaseRow struct {
	application.ModelRelease `gorm:"embedded"`
	Cost                     []byte
	Modes                    []byte
	Validated                []byte
}

const releaseSelect = `SELECT r.id,r.model_id,r.model_version_id,r.price_version_id,r.config_version_id,r.status,r.revision,r.upstream_cost AS cost,r.create_time AS created_at,
 to_jsonb(v.modes) AS modes,COALESCE((SELECT jsonb_agg(DISTINCT x.mode) FROM catalog.model_validation x
 JOIN catalog.provider_credential c ON c.id=x.credential_id AND c.status='active' AND NOT c.is_delete
 JOIN operation.operation op ON op.id=x.operation_id AND op.status='completed' AND NOT op.is_delete
 JOIN operation.provider_binding b ON b.operation_id=op.id AND b.release_id=x.release_id
 JOIN operation.provider_credential_binding k ON k.operation_id=op.id AND k.credential_id=x.credential_id
 WHERE op.mode=x.mode AND op.model_profile_version_id=r.model_version_id AND op.price_rule_version_id=r.price_version_id AND x.release_id=r.id AND x.status='succeeded' AND x.adapter_version='ai-provider-v1' AND x.evidence->>'kind'='real_provider'), '[]'::jsonb) AS validated
 FROM catalog.model_release r JOIN catalog.model_profile_version v ON v.id=r.model_version_id`

func readRelease(tx *gorm.DB, id uuid.UUID) (application.ModelRelease, error) {
	var row releaseRow
	r := tx.Raw(releaseSelect+` WHERE r.id=?`, id).Scan(&row)
	if r.Error != nil {
		return application.ModelRelease{}, r.Error
	}
	if r.RowsAffected != 1 {
		return application.ModelRelease{}, application.ErrModelNotFound
	}
	return decodeRelease(row)
}
func decodeRelease(row releaseRow) (application.ModelRelease, error) {
	if err := json.Unmarshal(row.Cost, &row.UpstreamCost); err != nil {
		return application.ModelRelease{}, err
	}
	if err := json.Unmarshal(row.Modes, &row.RequiredModes); err != nil {
		return application.ModelRelease{}, err
	}
	if err := json.Unmarshal(row.Validated, &row.ValidatedModes); err != nil {
		return application.ModelRelease{}, err
	}
	return row.ModelRelease, nil
}

// ListModelReleases reports evidence tied to the current credential, never guesses.
func (s *Store) ListModelReleases(ctx context.Context, actor identityapp.Principal, id uuid.UUID) ([]application.ModelRelease, error) {
	items := []application.ModelRelease{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requirePlatformCatalogAdmin(tx, s.actors, actor); err != nil {
			return err
		}
		var rows []releaseRow
		if err := tx.Raw(releaseSelect+` WHERE r.model_id=? ORDER BY r.create_time DESC,r.id LIMIT 200`, id).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			item, err := decodeRelease(row)
			if err != nil {
				return err
			}
			items = append(items, item)
		}
		return nil
	})
	return items, err
}

// CreateModelRelease accepts only versions belonging to the same provider/model.
func (s *Store) CreateModelRelease(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input application.CreateReleaseInput) (application.ModelRelease, error) {
	return catalogMutation(ctx, s.db, s.actors, actor, "release.create", id, input.RequestID, input, func(tx *gorm.DB) (application.ModelRelease, error) {
		var found int
		r := tx.Raw(`SELECT 1 FROM catalog.model_profile m
 JOIN catalog.model_profile_version v ON v.model_profile_id=m.id AND NOT v.is_delete
 JOIN catalog.price_rule_version price ON price.model_profile_id=m.id AND NOT price.is_delete
 JOIN catalog.provider_config_version config ON config.provider_id=m.provider_id
 JOIN catalog.provider_preset_version pv ON pv.id=config.preset_version_id
 JOIN catalog.provider_preset preset ON preset.id=pv.preset_id AND NOT preset.emergency_disabled
 WHERE m.id=? AND v.id=? AND price.id=? AND config.id=? AND NOT m.is_delete
 AND ((m.capability='text.generate' AND price.unit IN ('per_request','per_1k_tokens') AND v.modes <@ ARRAY['text_to_text']::text[])
 OR (m.capability='image.generate' AND price.unit IN ('per_request','per_image') AND v.modes <@ ARRAY['text2image','image2image']::text[])
 OR (m.capability='video.generate' AND price.unit IN ('per_request','per_second') AND v.modes <@ ARRAY['text2video','image2video','omni_reference']::text[]))
 AND (m.capability='image.generate' OR (v.limits->>'max_outputs')::int=1)
 AND v.supports_query=(m.capability='video.generate')
 AND (NOT v.limits ? 'provider_endpoint' OR (config.config->>'adapter_key'='openrouter' AND m.capability IN ('text.generate','image.generate')))
 AND v.queue='provider' AND v.moderation='provider' AND NOT v.supports_cancel AND NOT v.supports_callback
 AND ((m.capability='video.generate' AND config.config->>'adapter_key' IN ('openrouter','volcengine_ark'))
 OR (m.capability IN ('image.generate','text.generate') AND config.config->>'adapter_key'='openrouter'))
 FOR SHARE OF preset,m`, id, input.ModelVersionID, input.PriceVersionID, input.ConfigVersionID).Scan(&found)
		if r.Error != nil {
			return application.ModelRelease{}, r.Error
		}
		if r.RowsAffected != 1 {
			return application.ModelRelease{}, application.ErrModelSourceUnavailable
		}
		releaseID := uuid.New()
		cost, err := json.Marshal(input.UpstreamCost)
		if err != nil {
			return application.ModelRelease{}, err
		}
		if err := tx.Exec(`INSERT INTO catalog.model_release(id,model_id,model_version_id,price_version_id,config_version_id,upstream_cost) VALUES(?,?,?,?,?,?::jsonb)`, releaseID, id, input.ModelVersionID, input.PriceVersionID, input.ConfigVersionID, string(cost)).Error; err != nil {
			return application.ModelRelease{}, err
		}
		return readRelease(tx, releaseID)
	})
}

// ActivateModelRelease atomically switches the model and optional default after
// locking emergency controls, provider, credential and model in that order.
func (s *Store) ActivateModelRelease(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input application.ActivateReleaseInput) (application.ModelRelease, error) {
	return catalogMutation(ctx, s.db, s.actors, actor, "release.activate", id, input.RequestID, input, func(tx *gorm.DB) (application.ModelRelease, error) {
		var binding struct{ ModelID, ProviderID, ConfigID uuid.UUID }
		r := tx.Raw(`SELECT r.model_id,m.provider_id,r.config_version_id AS config_id FROM catalog.model_release r JOIN catalog.model_profile m ON m.id=r.model_id WHERE r.id=?`, id).Scan(&binding)
		if r.Error != nil {
			return application.ModelRelease{}, r.Error
		}
		if r.RowsAffected != 1 {
			return application.ModelRelease{}, application.ErrModelNotFound
		}
		var present int
		r = tx.Raw(`SELECT 1 FROM catalog.provider_preset p JOIN catalog.provider_preset_version v ON v.preset_id=p.id JOIN catalog.provider_config_version c ON c.preset_version_id=v.id WHERE c.id=? AND NOT p.emergency_disabled FOR SHARE OF p`, binding.ConfigID).Scan(&present)
		if r.Error != nil {
			return application.ModelRelease{}, r.Error
		}
		if r.RowsAffected != 1 {
			return application.ModelRelease{}, application.ErrPresetUnavailable
		}
		var provider struct{ Status string }
		r = tx.Raw(`SELECT status FROM catalog.provider WHERE id=? AND NOT is_delete FOR UPDATE`, binding.ProviderID).Scan(&provider)
		if r.Error != nil {
			return application.ModelRelease{}, r.Error
		}
		if r.RowsAffected != 1 || (!input.EnableProvider && provider.Status != "active") {
			return application.ModelRelease{}, application.ErrProviderUnavailable
		}
		var credential struct{ ID uuid.UUID }
		r = tx.Raw(`SELECT id FROM catalog.provider_credential WHERE provider_id=? AND status='active' AND NOT is_delete FOR SHARE`, binding.ProviderID).Scan(&credential)
		if r.Error != nil {
			return application.ModelRelease{}, r.Error
		}
		if r.RowsAffected != 1 {
			return application.ModelRelease{}, application.ErrCredentialNotFound
		}
		var model struct {
			Revision        int64
			ActiveReleaseID *uuid.UUID
			Capability      string
		}
		r = tx.Raw(`SELECT revision,active_release_id,capability FROM catalog.model_profile WHERE id=? AND NOT is_delete FOR UPDATE`, binding.ModelID).Scan(&model)
		if r.Error != nil {
			return application.ModelRelease{}, r.Error
		}
		if r.RowsAffected != 1 || model.Revision != input.ExpectedModelRevision || !equalNullableUUID(model.ActiveReleaseID, input.ExpectedActiveReleaseID) {
			return application.ModelRelease{}, application.ErrPresetConflict
		}
		var revision int64
		if err := tx.Raw(`SELECT revision FROM catalog.model_release WHERE id=? FOR UPDATE`, id).Scan(&revision).Error; err != nil {
			return application.ModelRelease{}, err
		}
		if revision != input.ExpectedRevision {
			return application.ModelRelease{}, application.ErrPresetConflict
		}
		var ready bool
		if err := tx.Raw(`SELECT r.status IN ('draft','retired') AND price.effective_from<=now() AND NOT price.is_delete AND NOT v.is_delete
 AND NOT EXISTS(SELECT 1 FROM unnest(v.modes) AS required(mode) WHERE NOT EXISTS(
 SELECT 1 FROM catalog.model_validation validation
 JOIN operation.operation op ON op.id=validation.operation_id AND op.status='completed' AND NOT op.is_delete
 JOIN operation.provider_binding execution ON execution.operation_id=op.id AND execution.release_id=validation.release_id
 JOIN operation.provider_credential_binding used_key ON used_key.operation_id=op.id AND used_key.credential_id=validation.credential_id
 WHERE validation.release_id=r.id AND validation.mode=required.mode AND validation.credential_id=? AND validation.status='succeeded'
 AND validation.adapter_version='ai-provider-v1' AND validation.evidence->>'kind'='real_provider'
 AND op.mode=validation.mode AND op.model_profile_version_id=r.model_version_id AND op.price_rule_version_id=r.price_version_id))
 FROM catalog.model_release r JOIN catalog.model_profile_version v ON v.id=r.model_version_id
 JOIN catalog.price_rule_version price ON price.id=r.price_version_id WHERE r.id=?`, credential.ID, id).Scan(&ready).Error; err != nil {
			return application.ModelRelease{}, err
		}
		if !ready {
			return application.ModelRelease{}, application.ErrReleaseUnverified
		}
		// Replacing a default release requires an explicit concurrent default switch.
		if !input.SetDefault && model.ActiveReleaseID != nil {
			var pinned bool
			if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM catalog.capability_default WHERE release_id=?)`, *model.ActiveReleaseID).Scan(&pinned).Error; err != nil {
				return application.ModelRelease{}, err
			}
			if pinned {
				return application.ModelRelease{}, application.ErrPresetConflict
			}
		}
		if input.SetDefault {
			if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "catalog.default/"+model.Capability).Error; err != nil {
				return application.ModelRelease{}, err
			}
			var current application.CapabilityDefault
			r := tx.Raw(`SELECT capability,release_id,revision FROM catalog.capability_default WHERE capability=? FOR UPDATE`, model.Capability).Scan(&current)
			if r.Error != nil {
				return application.ModelRelease{}, r.Error
			}
			if current.Revision != input.ExpectedDefaultRevision {
				return application.ModelRelease{}, application.ErrPresetConflict
			}
			if err := tx.Exec(`INSERT INTO catalog.capability_default(capability,release_id) VALUES(?,?) ON CONFLICT(capability) DO UPDATE SET release_id=EXCLUDED.release_id,revision=catalog.capability_default.revision+1,update_time=now()`, model.Capability, id).Error; err != nil {
				return application.ModelRelease{}, err
			}
		}
		if model.ActiveReleaseID != nil {
			if err := tx.Exec(`UPDATE catalog.model_release SET status='retired',revision=revision+1 WHERE id=?`, *model.ActiveReleaseID).Error; err != nil {
				return application.ModelRelease{}, err
			}
		}
		if err := tx.Exec(`UPDATE catalog.model_release SET status='active',revision=revision+1,activated_at=now() WHERE id=?`, id).Error; err != nil {
			return application.ModelRelease{}, err
		}
		if err := tx.Exec(`UPDATE catalog.model_profile SET active_release_id=?,status='active',revision=revision+1,update_time=now() WHERE id=?`, id, binding.ModelID).Error; err != nil {
			return application.ModelRelease{}, err
		}
		if input.EnableProvider {
			if err := tx.Exec(`UPDATE catalog.provider SET status='active',revision=revision+1,update_time=now() WHERE id=? AND status<>'active'`, binding.ProviderID).Error; err != nil {
				return application.ModelRelease{}, err
			}
		}
		return readRelease(tx, id)
	})
}
func equalNullableUUID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// ListCapabilityDefaults reads only explicit published defaults.
func (s *Store) ListCapabilityDefaults(ctx context.Context, actor identityapp.Principal) ([]application.CapabilityDefault, error) {
	items := []application.CapabilityDefault{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requirePlatformCatalogAdmin(tx, s.actors, actor); err != nil {
			return err
		}
		return tx.Raw(`SELECT capability,release_id,revision FROM catalog.capability_default ORDER BY capability`).Scan(&items).Error
	})
	if err != nil {
		return nil, fmt.Errorf("read capability defaults: %w", err)
	}
	return items, nil
}
