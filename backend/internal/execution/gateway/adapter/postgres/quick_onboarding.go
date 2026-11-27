package postgres

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/execution/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

const quickOnboardingPrefix = "quick-ark/"

func quickOnboardingGroup(scope uuid.UUID) string {
	return quickOnboardingPrefix + scope.String()
}

// PrepareFromPreset prepares one reusable keyless candidate from a reviewed catalogue version.
func (s *Store) PrepareFromPreset(ctx context.Context, req domain.QuickOnboardingInput) (OnboardingReview, error) {
	version, modelKey, ok := strings.Cut(req.PresetKey, "/")
	presetVersion, err := uuid.Parse(version)
	if !ok || err != nil || presetVersion == uuid.Nil || modelKey == "" || len(req.PresetKey) > 200 || req.ScopeID == uuid.Nil || req.ProjectID == uuid.Nil {
		return OnboardingReview{}, domain.ErrInvalid
	}
	var candidate uuid.UUID
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, s.actor.ScopeID, uuid.Nil, true); err != nil {
			return err
		}
		if err := s.check(ctx, tx, req.ScopeID, req.ProjectID, false); err != nil {
			return err
		}
		var row struct {
			Upstream                      string
			Driver, Estimate, QuickPreset []byte
		}
		result := tx.Raw(`SELECT m.payload->>'provider_model_id' AS upstream,
		 v.payload->'config' AS driver, price.video_estimate AS estimate, price.quick_onboarding AS quick_preset
		 FROM catalog.provider_preset p
		 JOIN LATERAL (SELECT id,payload FROM catalog.provider_preset_version WHERE preset_id=p.id ORDER BY version_no DESC LIMIT 1) v ON true
		 JOIN catalog.model_preset m ON m.preset_version_id=v.id
		 JOIN catalog.model_price_preset price ON price.adapter_key=m.payload->>'adapter_key'
		  AND price.provider_model_id=m.payload->>'provider_model_id'
		 WHERE v.id=? AND m.model_key=? AND p.origin='builtin' AND NOT p.archived AND NOT p.emergency_disabled
		  AND price.adapter_key='volcengine_ark' AND price.currency='CNY'
		  AND price.quick_onboarding IS NOT NULL AND price.video_estimate IS NOT NULL
		  AND m.payload->>'capability'='video.generate' AND m.payload->'modes' @> '["text2video"]'::jsonb
		  AND m.payload->>'lifecycle' IN ('active','alias') AND m.payload->>'requires_model_selection'='false'`,
			presetVersion, modelKey).Scan(&row)
		if result.Error != nil {
			return result.Error
		}
		var config catalog.ProviderConfig
		var profile domain.ArkVideoEstimateProfile
		var preset domain.ArkQuickOnboarding
		if result.RowsAffected != 1 || !catalog.IsSeedanceTokenModel(row.Upstream) ||
			json.Unmarshal(row.Driver, &config) != nil || config.AdapterKey != "volcengine_ark" ||
			json.Unmarshal(row.Estimate, &profile) != nil || json.Unmarshal(row.QuickPreset, &preset) != nil {
			return domain.ErrUnavailable
		}
		config.Capabilities = []string{"video"}
		config.ConcurrencyLimit, config.RateLimitPerMin = preset.Concurrency, preset.RPM
		if config.Validate() != nil {
			return domain.ErrInvalid
		}
		model, purchase, probe, err := preset.SeedanceDraft(req.PresetKey, row.Upstream, profile)
		if err != nil {
			return err
		}
		if err := lockAlias(tx, model.Name); err != nil {
			return err
		}
		group := quickOnboardingGroup(req.ScopeID)
		bound := New(tx, s.actor, s.authorize)
		var existing struct{ ID uuid.UUID }
		result = tx.Raw(`SELECT o.id FROM ai.onboarding o
		 WHERE o.model_id=? AND o.deleted_at IS NULL AND o.document->'route'->>'group' LIKE ?
		  AND (o.state='verifying' OR (o.state='active' AND `+activeOnboardingRoute+`)
		   OR (o.state='prepared' AND o.document->>'scope_id'=? AND o.document->>'project_id'=?))
		 ORDER BY CASE o.state WHEN 'active' THEN 0 WHEN 'verifying' THEN 1 ELSE 2 END,
		  o.created_at,o.id LIMIT 1`, model.ID, quickOnboardingPrefix+"%", req.ScopeID.String(), req.ProjectID.String()).Scan(&existing)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			candidate = existing.ID
			return nil
		}
		if err := bound.StageModel(ctx, model); err != nil {
			return err
		}
		// A rolled-back release can be verified again using the same immutable terms.
		if err := tx.Exec("UPDATE ai.model SET draft=true WHERE id=? AND NOT enabled AND NOT draft", model.ID).Error; err != nil {
			return err
		}
		candidate = uuid.New()
		return bound.PrepareOnboarding(ctx, application.Onboarding{
			Scope: req.ScopeID, Project: req.ProjectID, Concurrency: preset.Concurrency, RPM: preset.RPM,
			MediaTests: []json.RawMessage{mustJSON(probe)},
			Route: domain.Route{ID: candidate, ModelID: model.ID, Supplier: uuid.NewSHA1(uuid.NameSpaceURL, []byte(group)),
				Group: group, BaseURL: config.BaseURL, UpstreamModel: row.Upstream, Protocol: model.Protocol,
				Purchase: purchase, DriverConfig: mustJSON(config)},
		})
	})
	if err != nil {
		return OnboardingReview{}, err
	}
	return s.ReviewOnboarding(ctx, candidate)
}
