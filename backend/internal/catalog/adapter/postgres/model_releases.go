package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/paramvalidation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/pricevalidation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

type releaseRow struct {
	application.ModelRelease `gorm:"embedded"`
	Cost                     []byte
	Modes                    []byte
	Validated                []byte
	DisplayName              string
	BatchDraft               bool
}

const releaseFields = `r.onboarding_check_id,r.id,r.model_id,r.model_version_id,r.price_version_id,r.config_version_id,r.status,r.revision,r.upstream_cost AS cost,r.create_time AS created_at,
 to_jsonb(v.modes) AS modes,COALESCE((SELECT jsonb_agg(DISTINCT x.mode) FROM catalog.model_validation x
 JOIN catalog.provider_credential c ON c.id=x.credential_id AND c.status='active' AND NOT c.is_delete
 JOIN catalog.provider_config_version cfg ON cfg.id=r.config_version_id AND cfg.provider_id=c.provider_id
 JOIN catalog.provider p ON p.id=cfg.provider_id AND p.current_config_version_id=cfg.id AND NOT p.is_delete
 JOIN operation.operation op ON op.id=x.operation_id AND op.status='completed' AND NOT op.is_delete
 JOIN operation.provider_binding b ON b.operation_id=op.id AND b.release_id=x.release_id
 JOIN operation.provider_credential_binding k ON k.operation_id=op.id AND k.credential_id=x.credential_id
 WHERE op.mode=x.mode AND op.model_profile_version_id=r.model_version_id AND op.price_rule_version_id=r.price_version_id AND x.release_id=r.id AND x.status='succeeded' AND x.adapter_version='ai-provider-v1' AND x.evidence->>'kind'='real_provider'), '[]'::jsonb) AS validated`

const releaseFrom = ` FROM catalog.model_release r JOIN catalog.model_profile_version v ON v.id=r.model_version_id`
const releaseSelect = `SELECT ` + releaseFields + releaseFrom

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
		if err := tx.Raw(releaseSelect+` WHERE r.model_id=? AND EXISTS (SELECT 1 FROM catalog.model_profile m WHERE m.id=r.model_id AND NOT m.is_delete) ORDER BY r.create_time DESC,r.id LIMIT 200`, id).Scan(&rows).Error; err != nil {
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
 OR (m.capability='audio.generate' AND config.config->>'adapter_key'='volcengine_speech' AND v.provider_model_id='seed-tts-2.0' AND v.modes=ARRAY['text2audio']::text[] AND price.unit IN ('per_request','per_1k_chars') AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(v.param_schema) field WHERE field->>'field' NOT IN ('speaker','speech_rate','loudness_rate')))
 OR (m.capability='audio.generate' AND config.config->>'adapter_key'='volcengine_speech' AND v.provider_model_id='seed-audio-1.0' AND v.modes=ARRAY['omni_audio']::text[] AND price.unit IN ('per_request','per_second') AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(v.param_schema) field WHERE field->>'field' NOT IN ('speech_rate','loudness_rate','pitch_rate','enable_subtitle')))
 OR (m.capability='audio.generate' AND config.config->>'adapter_key'='volcengine_music' AND v.provider_model_id='v5.0' AND v.modes <@ ARRAY['music_song','music_instrumental']::text[] AND price.unit IN ('per_request','per_second') AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(v.param_schema) field WHERE field->>'field' NOT IN ('duration_ms','music_input')))
 OR (m.capability='image.generate' AND price.unit IN ('per_request','per_image') AND v.modes <@ ARRAY['text2image','image2image']::text[])
 OR (m.capability='video.generate' AND
   (price.unit IN ('per_request','per_second') OR
    (price.unit='per_1k_tokens' AND config.config->>'adapter_key'='volcengine_ark'
     AND v.provider_model_id LIKE 'doubao-seedance-2-%' AND price.rule->>'input_micros_per_1k'='0'))
   AND v.modes <@ ARRAY['text2video','image2video','omni_reference','avatar','motion']::text[]))
 AND (m.capability='image.generate' OR (v.limits->>'max_outputs')::int=1)
 AND v.supports_query=(m.capability='video.generate' OR config.config->>'adapter_key'='volcengine_music')
 AND (NOT v.limits ? 'provider_endpoint' OR (config.config->>'adapter_key'='openrouter' AND m.capability IN ('text.generate','image.generate')))
 AND v.queue='provider' AND v.moderation='provider' AND NOT v.supports_cancel AND NOT v.supports_callback
 AND ((m.capability='video.generate' AND config.config->>'adapter_key' IN ('openrouter','volcengine_ark'))
 OR (m.capability='video.generate' AND config.config->>'adapter_key'='volcengine_avatar' AND v.provider_model_id='jimeng_realman_avatar_picture_omni_v15' AND v.modes=ARRAY['avatar']::text[] AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(v.param_schema) field WHERE field->>'field' NOT IN ('output_resolution','pe_fast_mode')))
 OR (m.capability='video.generate' AND config.config->>'adapter_key'='volcengine_motion' AND v.provider_model_id='jimeng_dreamactor_m20_gen_video' AND v.modes=ARRAY['motion']::text[] AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(v.param_schema) field WHERE field->>'field' NOT IN ('cut_result_first_second_switch')))
 OR (m.capability='text.generate' AND config.config->>'adapter_key' IN ('openrouter','volcengine_ark','byteplus','deepseek','openai','vercel'))
 OR (m.capability='audio.generate' AND config.config->>'adapter_key' IN ('volcengine_speech','volcengine_music'))
 OR (m.capability='image.generate' AND config.config->>'adapter_key' IN ('openai','vercel') AND v.modes=ARRAY['text2image']::text[] AND (v.limits->>'max_outputs')::int=1 AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(v.param_schema) field WHERE field->>'field' NOT IN ('size','quality')))
 OR (m.capability='image.generate' AND (config.config->>'adapter_key'='openrouter' OR (config.config->>'adapter_key' IN ('volcengine_ark','byteplus') AND (v.limits->>'max_outputs')::int=1))))
 FOR SHARE OF preset,m`, id, input.ModelVersionID, input.PriceVersionID, input.ConfigVersionID).Scan(&found)
		if r.Error != nil {
			return application.ModelRelease{}, r.Error
		}
		if r.RowsAffected != 1 {
			return application.ModelRelease{}, application.ErrModelSourceUnavailable
		}
		if err := validateReleaseContract(tx, id, input); err != nil {
			return application.ModelRelease{}, err
		}
		releaseID := uuid.New()
		cost := []byte("{}")
		if input.UpstreamCost != (application.UpstreamCost{}) {
			var err error
			cost, err = json.Marshal(input.UpstreamCost)
			if err != nil {
				return application.ModelRelease{}, err
			}
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
		var provider struct {
			Status, AdapterKey     string
			CurrentConfigVersionID uuid.UUID
		}
		r = tx.Raw(`SELECT status,adapter_key,current_config_version_id FROM catalog.provider WHERE id=? AND NOT is_delete FOR UPDATE`, binding.ProviderID).Scan(&provider)
		if r.Error != nil {
			return application.ModelRelease{}, r.Error
		}
		if r.RowsAffected != 1 || provider.Status == "archived" || (!input.EnableProvider && provider.Status != "active") {
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
			Status          string
		}
		r = tx.Raw(`SELECT revision,active_release_id,capability,status FROM catalog.model_profile WHERE id=? AND NOT is_delete FOR UPDATE`, binding.ModelID).Scan(&model)
		if r.Error != nil {
			return application.ModelRelease{}, r.Error
		}
		if r.RowsAffected != 1 || model.Status == "archived" || model.Revision != input.ExpectedModelRevision || !equalNullableUUID(model.ActiveReleaseID, input.ExpectedActiveReleaseID) {
			return application.ModelRelease{}, application.ErrPresetConflict
		}
		var revision int64
		if err := tx.Raw(`SELECT revision FROM catalog.model_release WHERE id=? FOR UPDATE`, id).Scan(&revision).Error; err != nil {
			return application.ModelRelease{}, err
		}
		if revision != input.ExpectedRevision {
			return application.ModelRelease{}, application.ErrPresetConflict
		}
		isCurrentRelease := model.ActiveReleaseID != nil && *model.ActiveReleaseID == id
		resumeCurrent := isCurrentRelease && (model.Status == "disabled" || provider.Status == "disabled")
		var eligible bool
		if err := tx.Raw(`SELECT (r.status IN ('draft','retired') OR (r.status='active' AND ?))
 AND price.effective_from<=now() AND NOT price.is_delete AND NOT v.is_delete
 FROM catalog.model_release r JOIN catalog.model_profile_version v ON v.id=r.model_version_id
 JOIN catalog.price_rule_version price ON price.id=r.price_version_id WHERE r.id=?`, resumeCurrent, id).Scan(&eligible).Error; err != nil {
			return application.ModelRelease{}, err
		}
		if !eligible {
			return application.ModelRelease{}, application.ErrReleaseUnverified
		}
		release, err := readRelease(tx, id)
		if err != nil {
			return application.ModelRelease{}, err
		}
		if input.CredentialCheckID == nil {
			if !allReleaseModesVerified(release) {
				return application.ModelRelease{}, application.ErrReleaseUnverified
			}
		} else {
			// Older Vercel checks used the public model catalog. Only the
			// newer auth-only probe completes without directory discovery.
			vercel := provider.AdapterKey == "vercel"
			var verified bool
			if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM catalog.provider_check
 WHERE id=? AND provider_id=? AND config_version_id=? AND credential_id=?
 AND kind='credential_auth' AND status='completed' AND auth_result='ok' AND adapter_result='ok'
 AND completed_at IS NOT NULL AND (NOT ? OR discovery_result='unsupported'))`,
				*input.CredentialCheckID, binding.ProviderID, binding.ConfigID, credential.ID, vercel).Scan(&verified).Error; err != nil {
				return application.ModelRelease{}, err
			}
			if !verified || provider.CurrentConfigVersionID != binding.ConfigID {
				return application.ModelRelease{}, application.ErrReleaseUnverified
			}
			if err := validateReleaseContract(tx, binding.ModelID, application.CreateReleaseInput{
				ModelVersionID: release.ModelVersionID, PriceVersionID: release.PriceVersionID, ConfigVersionID: release.ConfigVersionID,
			}); err != nil {
				return application.ModelRelease{}, err
			}
		}
		if input.EnableProvider && provider.Status != "active" {
			if err := requireReviewedLegacyReleases(tx, binding.ProviderID, binding.ModelID); err != nil {
				return application.ModelRelease{}, err
			}
		}
		// Replacing a default release requires an explicit concurrent default switch.
		if !input.SetDefault && model.ActiveReleaseID != nil && !isCurrentRelease {
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
		if model.ActiveReleaseID != nil && !isCurrentRelease {
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
func allReleaseModesVerified(release application.ModelRelease) bool {
	return len(release.RequiredModes) > 0 && !slices.ContainsFunc(release.RequiredModes, func(mode string) bool {
		return !slices.Contains(release.ValidatedModes, mode)
	})
}

// A disabled connection cannot restore unrelated historical releases via a verified sibling.
func requireReviewedLegacyReleases(tx *gorm.DB, provider, replacingModel uuid.UUID) error {
	var rows []releaseRow
	if err := tx.Raw(releaseSelect+` JOIN catalog.model_profile m ON m.active_release_id=r.id
 WHERE m.provider_id=? AND m.id<>? AND m.status='active' AND NOT m.is_delete
 AND r.status='active' AND r.onboarding_check_id IS NOT NULL`, provider, replacingModel).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		release, err := decodeRelease(row)
		if err != nil {
			return err
		}
		if !allReleaseModesVerified(release) {
			return application.ErrReleaseUnverified
		}
	}
	return nil
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

// validateReleaseContract shares portable-template checks with releases assembled
// from separately published versions. It runs inside the release transaction.
func validateReleaseContract(tx *gorm.DB, modelID uuid.UUID, input application.CreateReleaseInput) error {
	var row struct {
		ID, ModelID                                     uuid.UUID
		VersionNo                                       int
		ProviderModelID                                 string
		ModesJSON, ConfigJSON, Limits, ParamSchema      []byte
		Capability, Queue                               string
		Moderation                                      domain.Moderation
		SupportsQuery, SupportsCancel, SupportsCallback bool
		ExpectedMaxMS                                   int
	}
	r := tx.Raw(`SELECT v.id,v.model_profile_id AS model_id,v.version_no,v.provider_model_id,to_jsonb(v.modes) AS modes_json,
 v.limits,v.param_schema,v.supports_query,v.supports_cancel,v.supports_callback,v.expected_max_ms,v.moderation,v.queue,
 config.config AS config_json,m.capability
 FROM catalog.model_profile_version v JOIN catalog.model_profile m ON m.id=v.model_profile_id
 JOIN catalog.provider_config_version config ON config.provider_id=m.provider_id
 WHERE v.id=? AND m.id=? AND config.id=?`, input.ModelVersionID, modelID, input.ConfigVersionID).Scan(&row)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return application.ErrModelSourceUnavailable
	}
	version := domain.ModelVersion{
		ID: row.ID, ModelID: row.ModelID, VersionNo: row.VersionNo, ProviderModelID: row.ProviderModelID,
		Limits: row.Limits, ParamSchema: row.ParamSchema, ExpectedMaxMS: row.ExpectedMaxMS,
		Queue: row.Queue, Moderation: row.Moderation, SupportsQuery: row.SupportsQuery,
		SupportsCancel: row.SupportsCancel, SupportsCallback: row.SupportsCallback,
	}
	var config domain.ProviderConfig
	if json.Unmarshal(row.ModesJSON, &version.Modes) != nil || json.Unmarshal(row.ConfigJSON, &config) != nil {
		return application.ErrModelSourceUnavailable
	}
	price, err := readReleasePrice(tx, modelID, input.PriceVersionID)
	if err != nil {
		return err
	}
	params, err := paramvalidation.NewValidator()
	if err != nil {
		return err
	}
	if params.Validate(version) != nil || pricevalidation.ValidateModel(config, row.Capability, version, price) != nil {
		return application.ErrModelSourceUnavailable
	}
	procurementID, err := price.ProcurementVersion()
	if err != nil {
		return application.ErrModelSourceUnavailable
	}
	if procurementID == uuid.Nil {
		return nil
	}
	// GPT Image 2.5 requires a token meter that this image adapter has not
	// published yet. A flat retail rule cannot stand in for supplier evidence.
	if version.ProviderModelID == "gpt-image-2.5-sunburst" || version.ProviderModelID == "gpt-image-2.5-flare" {
		return application.ErrModelSourceUnavailable
	}
	procurement, err := readReleasePrice(tx, modelID, procurementID)
	if err != nil {
		return err
	}
	if pricevalidation.ValidateProcurement(price, procurement) != nil ||
		pricevalidation.ValidateModel(config, row.Capability, version, procurement) != nil {
		return application.ErrModelSourceUnavailable
	}
	return nil
}

func readReleasePrice(tx *gorm.DB, modelID, priceID uuid.UUID) (domain.PriceRuleVersion, error) {
	// Price versions are append-only for the application role. Release creation
	// already locks the model; a row lock here would require price UPDATE access.
	var price domain.PriceRuleVersion
	r := tx.Raw(`SELECT id,model_profile_id AS model_id,version_no,unit,rule,currency,
 fx_rate_to_cny::text AS fx_rate_to_cny,effective_from,source_url,reviewed_at
 FROM catalog.price_rule_version WHERE id=? AND model_profile_id=? AND NOT is_delete`, priceID, modelID).Scan(&price)
	if r.Error != nil {
		return price, r.Error
	}
	if r.RowsAffected != 1 {
		return price, application.ErrModelSourceUnavailable
	}
	return price, nil
}
