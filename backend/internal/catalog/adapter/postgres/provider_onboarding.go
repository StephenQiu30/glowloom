package postgres

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/paramvalidation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func instantiateOnboardingModel(tx *gorm.DB, actor identityapp.Principal, id uuid.UUID, expectedRevision int64, candidate domain.ModelCandidatePreset) error {
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
	if err := appendModelVersionInTx(tx, version, expectedRevision); err != nil {
		return err
	}
	return nil
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
 FROM catalog.model_profile m
 JOIN catalog.provider p ON p.id=m.provider_id
 JOIN catalog.model_profile_version v ON v.id=m.current_version_id
 LEFT JOIN catalog.model_release r ON r.model_id=m.id AND r.model_version_id=v.id AND r.config_version_id=p.current_config_version_id

 WHERE m.provider_id=? AND m.status<>'archived' AND m.candidate_snapshot IS NOT NULL AND NOT m.is_delete AND NOT v.is_delete AND (r.id IS NULL OR r.upstream_cost='{}'::jsonb)
 ORDER BY m.capability,m.display_name LIMIT ?`, id, domain.MaxPresetModels).Scan(&result.Models).Error; err != nil {
			return err
		}
		reviews, err := readOnboardingReviews(tx, id)
		if err != nil {
			return err
		}
		result.ReleaseReviews = reviews
		var check application.ProviderCheck
		r = tx.Raw(`SELECT check_run.id,check_run.provider_id,check_run.config_version_id,check_run.credential_id,check_run.model_id,check_run.model_version_id,
 check_run.elapsed_ms,check_run.request_id,check_run.tracking_ended_at,check_run.tracking_note,check_run.kind,check_run.status,check_run.auth_result,check_run.adapter_result,check_run.discovery_result,check_run.model_result,check_run.task_id,check_run.error_code,check_run.create_time AS created_at,check_run.completed_at,true AS current
 FROM catalog.provider_check check_run JOIN catalog.provider p ON p.id=check_run.provider_id AND p.current_config_version_id=check_run.config_version_id
 JOIN catalog.provider_credential c ON c.id=check_run.credential_id AND c.status='active' AND NOT c.is_delete
 JOIN catalog.model_profile checked_model ON checked_model.id=check_run.model_id AND checked_model.current_version_id=check_run.model_version_id AND NOT checked_model.is_delete
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

// ActivateProviderOnboarding activates the batch only after verifying every required mode.
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
		type batchModel struct {
			ID, VersionID, ReleaseID uuid.UUID
			Revision                 int64
			Status, ReleaseStatus    string
		}
		var models []batchModel
		if err := tx.Raw(`SELECT m.id,m.revision,m.status,m.current_version_id AS version_id,r.id AS release_id,r.status AS release_status
 FROM catalog.model_profile m JOIN catalog.model_release r ON r.model_id=m.id AND r.model_version_id=m.current_version_id
 JOIN catalog.model_profile_version v ON v.id=r.model_version_id AND NOT v.is_delete
 JOIN catalog.price_rule_version price ON price.id=r.price_version_id AND NOT price.is_delete AND price.effective_from<=now()
 WHERE m.provider_id=? AND m.status='disabled' AND m.candidate_snapshot IS NOT NULL AND NOT m.is_delete AND r.config_version_id=? AND r.upstream_cost='{}'::jsonb AND r.status='draft'
 ORDER BY m.id FOR UPDATE OF m,r`, id, provider.ConfigID).Scan(&models).Error; err != nil {
			return empty, err
		}
		if len(models) == 0 || len(models) > domain.MaxPresetModels {
			return empty, domain.ErrInvalidPreset
		}
		if len(input.ReleaseIDs) != 0 && (len(input.ReleaseIDs) != len(models) || slices.ContainsFunc(models, func(model batchModel) bool { return !slices.Contains(input.ReleaseIDs, model.ReleaseID) })) {
			return empty, application.ErrPresetConflict
		}
		if err := requireReviewedLegacyReleases(tx, id, uuid.Nil); err != nil {
			return empty, err
		}
		for _, model := range models {
			if model.Revision != 3 || model.Status != "disabled" || model.ReleaseStatus != "draft" {
				return empty, application.ErrPresetConflict
			}
		}
		for _, model := range models {
			release, err := readRelease(tx, model.ReleaseID)
			if err != nil {
				return empty, err
			}
			if !allReleaseModesVerified(release) {
				return empty, application.ErrReleaseUnverified
			}
		}
		for _, model := range models {
			if err := tx.Exec(`UPDATE catalog.model_release SET status='active',revision=revision+1,activated_at=now() WHERE id=?`, model.ReleaseID).Error; err != nil {
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

func readOnboardingReviews(tx *gorm.DB, id uuid.UUID) ([]application.OnboardingReleaseReview, error) {
	const batchDraft = `r.status='draft' AND m.status='disabled' AND m.revision=3 AND m.candidate_snapshot IS NOT NULL
 AND m.current_version_id=r.model_version_id AND p.current_config_version_id=r.config_version_id
 AND r.upstream_cost='{}'::jsonb AND NOT v.is_delete AND NOT price.is_delete AND price.effective_from<=now()`
	query := `SELECT ` + releaseFields + `,m.display_name,(` + batchDraft + `) AS batch_draft` + releaseFrom
	var rows []releaseRow
	err := tx.Raw(query+` JOIN catalog.model_profile m ON m.id=r.model_id
 JOIN catalog.provider p ON p.id=m.provider_id
 JOIN catalog.price_rule_version price ON price.id=r.price_version_id
 WHERE p.id=? AND NOT m.is_delete AND (r.onboarding_check_id IS NOT NULL OR (`+batchDraft+`))
 ORDER BY batch_draft DESC,m.display_name,r.create_time DESC,r.id LIMIT 200`, id).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	reviews := make([]application.OnboardingReleaseReview, 0, len(rows))
	for _, row := range rows {
		release, err := decodeRelease(row)
		if err != nil {
			return nil, err
		}
		reviews = append(reviews, application.OnboardingReleaseReview{ModelRelease: release, DisplayName: row.DisplayName, BatchDraft: row.BatchDraft})
	}
	return reviews, nil
}
