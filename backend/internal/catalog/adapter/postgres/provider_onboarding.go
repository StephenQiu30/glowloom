package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/paramvalidation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func instantiateOnboardingModel(tx *gorm.DB, actor identityapp.Principal, id, configID uuid.UUID, candidate domain.ModelCandidatePreset) error {
	version, err := candidate.OnboardingVersion(id, actor.ID)
	if err != nil {
		return err
	}
	validator, err := paramvalidation.NewValidator()
	if err != nil {
		return err
	}
	if err := validator.Validate(version); err != nil {
		return err
	}
	if err := appendModelVersionInTx(tx, version, 1); err != nil {
		return err
	}
	// This is the platform's zero surcharge, not an estimate of supplier costs.
	price := domain.PriceRuleVersion{ID: uuid.New(), ModelID: id, VersionNo: 1, Unit: domain.PricePerRequest, Rule: json.RawMessage(`{"base_micros":0}`), Currency: "CNY", EffectiveFrom: time.Now().UTC(), CreateBy: actor.ID}
	if err := appendPriceRuleInTx(tx, price, 2); err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO catalog.model_release(id,model_id,model_version_id,price_version_id,config_version_id,upstream_cost) VALUES(?,?,?,?,?, '{}'::jsonb)`, uuid.New(), id, version.ID, price.ID, configID).Error
}

// ReadProviderOnboarding reads only safe saved selections and current check evidence.
func (s *Store) ReadProviderOnboarding(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (application.ProviderOnboardingState, error) {
	result := application.ProviderOnboardingState{Models: []application.OnboardingModel{}}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requirePlatformAdmin(tx, actor); err != nil {
			return err
		}
		r := tx.Raw(`SELECT p.id AS provider_id,p.revision,p.status,c.id AS credential_id FROM catalog.provider p
 LEFT JOIN catalog.provider_credential c ON c.provider_id=p.id AND c.status='active' AND NOT c.is_delete
 WHERE p.id=? AND NOT p.is_delete`, id).Scan(&result)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrProviderNotFound
		}
		if err := tx.Raw(`SELECT m.id,v.id AS version_id,r.id AS release_id,m.display_name,v.provider_model_id,m.capability,m.status
 FROM catalog.model_profile m JOIN catalog.model_release r ON r.model_id=m.id AND r.upstream_cost='{}'::jsonb
 JOIN catalog.model_profile_version v ON v.id=r.model_version_id
 JOIN catalog.provider p ON p.id=m.provider_id AND p.current_config_version_id=r.config_version_id
 WHERE m.provider_id=? AND m.candidate_snapshot IS NOT NULL AND NOT m.is_delete AND NOT v.is_delete
 ORDER BY m.capability,m.display_name LIMIT 16`, id).Scan(&result.Models).Error; err != nil {
			return err
		}
		var check application.ProviderCheck
		r = tx.Raw(`SELECT check_run.id,check_run.provider_id,check_run.config_version_id,check_run.credential_id,check_run.model_id,check_run.model_version_id,
 check_run.kind,check_run.status,check_run.auth_result,check_run.adapter_result,check_run.discovery_result,check_run.model_result,check_run.task_id,check_run.error_code,check_run.create_time AS created_at,check_run.completed_at
 FROM catalog.provider_check check_run JOIN catalog.provider p ON p.id=check_run.provider_id AND p.current_config_version_id=check_run.config_version_id
 JOIN catalog.provider_credential c ON c.id=check_run.credential_id AND c.status='active' AND NOT c.is_delete
 WHERE check_run.provider_id=? AND check_run.kind='model_access' ORDER BY check_run.create_time DESC,check_run.id DESC LIMIT 1`, id).Scan(&check)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 1 {
			result.Check = &check
		}
		return nil
	})
	return result, err
}

// ActivateProviderOnboarding pins all originally selected models atomically. It
// never creates per-mode inference evidence for models that were not tested.
func (s *Store) ActivateProviderOnboarding(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input application.ActivateOnboardingInput) (application.ProviderOnboardingState, error) {
	return catalogMutation(ctx, s.db, s.actors, actor, "provider.onboarding.activate", id, input.RequestID, input, func(tx *gorm.DB) (application.ProviderOnboardingState, error) {
		empty := application.ProviderOnboardingState{}
		var present int
		r := tx.Raw(`SELECT 1 FROM catalog.provider p JOIN catalog.provider_config_version config ON config.id=p.current_config_version_id
 JOIN catalog.provider_preset_version pv ON pv.id=config.preset_version_id JOIN catalog.provider_preset preset ON preset.id=pv.preset_id
 WHERE p.id=? AND NOT preset.emergency_disabled FOR SHARE OF preset`, id).Scan(&present)
		if r.Error != nil {
			return empty, r.Error
		}
		if r.RowsAffected != 1 {
			return empty, application.ErrPresetUnavailable
		}
		var provider struct {
			Revision int64
			ConfigID uuid.UUID
			Status   string
		}
		r = tx.Raw(`SELECT revision,current_config_version_id AS config_id,status FROM catalog.provider WHERE id=? AND NOT is_delete FOR UPDATE`, id).Scan(&provider)
		if r.Error != nil {
			return empty, r.Error
		}
		if r.RowsAffected != 1 || provider.Revision != input.ExpectedRevision || provider.Status != "disabled" {
			return empty, application.ErrPresetConflict
		}
		var credential struct{ ID uuid.UUID }
		r = tx.Raw(`SELECT id FROM catalog.provider_credential WHERE provider_id=? AND status='active' AND NOT is_delete FOR SHARE`, id).Scan(&credential)
		if r.Error != nil {
			return empty, r.Error
		}
		if r.RowsAffected != 1 {
			return empty, application.ErrCredentialNotFound
		}
		var models []struct {
			ID, VersionID, ReleaseID uuid.UUID
			Revision                 int64
			Status, ReleaseStatus    string
		}
		if err := tx.Raw(`SELECT m.id,m.revision,m.status,m.current_version_id AS version_id,r.id AS release_id,r.status AS release_status
 FROM catalog.model_profile m JOIN catalog.model_release r ON r.model_id=m.id AND r.model_version_id=m.current_version_id
 JOIN catalog.model_profile_version v ON v.id=r.model_version_id AND NOT v.is_delete
 JOIN catalog.price_rule_version price ON price.id=r.price_version_id AND NOT price.is_delete AND price.effective_from<=now()
 WHERE m.provider_id=? AND m.candidate_snapshot IS NOT NULL AND NOT m.is_delete AND r.config_version_id=? AND r.upstream_cost='{}'::jsonb
 ORDER BY m.id FOR UPDATE OF m,r`, id, provider.ConfigID).Scan(&models).Error; err != nil {
			return empty, err
		}
		if len(models) == 0 || len(models) > 16 {
			return empty, domain.ErrInvalidPreset
		}
		for _, model := range models {
			if model.Revision != 3 || model.Status != "disabled" || model.ReleaseStatus != "draft" {
				return empty, application.ErrPresetConflict
			}
		}
		var check struct{ ModelID, VersionID uuid.UUID }
		r = tx.Raw(`SELECT model_id,model_version_id AS version_id FROM catalog.provider_check
 WHERE id=? AND provider_id=? AND config_version_id=? AND credential_id=? AND kind='model_access'
 AND status='completed' AND auth_result='ok' AND model_result IN ('completed','accepted') AND dispatch_started_at IS NOT NULL FOR SHARE`, input.CheckID, id, provider.ConfigID, credential.ID).Scan(&check)
		if r.Error != nil {
			return empty, r.Error
		}
		if r.RowsAffected != 1 {
			return empty, application.ErrReleaseUnverified
		}
		found := false
		for _, model := range models {
			if model.ID == check.ModelID && model.VersionID == check.VersionID {
				found = true
			}
		}
		if !found {
			return empty, application.ErrReleaseUnverified
		}
		for _, model := range models {
			if err := tx.Exec(`UPDATE catalog.model_release SET status='active',onboarding_check_id=?,revision=revision+1,activated_at=now() WHERE id=?`, input.CheckID, model.ReleaseID).Error; err != nil {
				return empty, err
			}
			if err := tx.Exec(`UPDATE catalog.model_profile SET status='active',active_release_id=?,revision=revision+1,update_time=now() WHERE id=?`, model.ReleaseID, model.ID).Error; err != nil {
				return empty, err
			}
		}
		if err := tx.Exec(`UPDATE catalog.provider SET status='active',revision=revision+1,update_time=now() WHERE id=?`, id).Error; err != nil {
			return empty, err
		}
		return s.withDB(tx).ReadProviderOnboarding(ctx, actor, id)
	})
}
