package postgres

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func recordProviderCheck(tx *gorm.DB, observation application.CredentialTestObservation) error {
	if observation.AuthResult == "" {
		return nil
	} // Existing legacy test workflows have no pinned check.
	allowed := []string{"ok", "unknown", "unsupported", "unreachable", "auth_failed", "insufficient_balance", "rate_limited", "invalid_parameters", "not_found", "upstream_unavailable", "partial", "invalid_response", "insufficient_quota", "project_limit_exceeded", "organization_limit_exceeded", "ip_restricted", "region_restricted", "permission_denied"}
	if !slices.Contains(allowed, observation.AuthResult) || !slices.Contains(allowed, observation.AdapterResult) || !slices.Contains(allowed, observation.DiscoveryResult) || len(observation.Candidates) > 600 {
		return application.ErrInvalidCredentialTest
	}
	if !slices.Contains([]string{"", "completed", "accepted", "rejected", "unknown"}, observation.ModelResult) || len(observation.TaskID) > 256 {
		return application.ErrInvalidCredentialTest
	}
	status := "completed"
	if observation.AuthResult != "ok" && observation.AuthResult != "unsupported" {
		status = "failed"
	}
	r := tx.Exec(`UPDATE catalog.provider_check SET status=?,auth_result=?,adapter_result=?,discovery_result=?,completed_at=?,model_result=?,task_id=?,http_status=?,elapsed_ms=? WHERE id=? AND provider_id=? AND credential_id=?`, status, observation.AuthResult, observation.AdapterResult, observation.DiscoveryResult, observation.TestedAt, observation.ModelResult, observation.TaskID, observation.HTTPStatus, observation.ElapsedMS, observation.TestID, observation.ProviderID, observation.CredentialID)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return application.ErrInvalidCredentialTest
	}
	if err := tx.Exec(`UPDATE catalog.provider_check original SET status=?,auth_result=?,model_result=?,completed_at=? FROM catalog.provider_check query WHERE query.id=? AND query.kind='model_query' AND original.id=query.parent_check_id AND original.config_version_id=query.config_version_id AND original.credential_id=query.credential_id AND original.task_id=query.task_id`, status, observation.AuthResult, observation.ModelResult, observation.TestedAt, observation.TestID).Error; err != nil {
		return err
	}
	for _, candidate := range observation.Candidates {
		if candidate.ProviderModelID == "" || len(candidate.ProviderModelID) > 256 || len(candidate.DisplayName) > 300 || len(candidate.Metadata) > 2048 || !json.Valid(candidate.Metadata) || !slices.Contains([]string{"text.generate", "image.generate", "video.generate"}, candidate.Capability) {
			return application.ErrInvalidCredentialTest
		}
		if err := tx.Exec(`INSERT INTO catalog.model_candidate(id,check_id,provider_model_id,display_name,capability,metadata) VALUES(?,?,?,?,?,?::jsonb) ON CONFLICT(check_id,provider_model_id,capability) DO NOTHING`, uuid.New(), observation.TestID, candidate.ProviderModelID, candidate.DisplayName, candidate.Capability, string(candidate.Metadata)).Error; err != nil {
			return err
		}
	}
	return nil
}

// ReadProviderCheck returns safe evidence and bounded discovery candidates.
func (s *Store) ReadProviderCheck(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (application.ProviderCheck, error) {
	var result application.ProviderCheck
	err := taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		if err := requirePlatformCatalogAdmin(tx, s.actors, actor); err != nil {
			return err
		}
		r := tx.Raw(`SELECT check_run.*,check_run.create_time AS created_at,
 (p.current_config_version_id=check_run.config_version_id AND NOT p.is_delete AND
 (check_run.kind='reachability' OR (credential.status='active' AND NOT credential.is_delete)) AND
 (check_run.model_id IS NULL OR (model.current_version_id=check_run.model_version_id AND NOT model.is_delete))) AS current FROM catalog.provider_check check_run JOIN catalog.provider p ON p.id=check_run.provider_id LEFT JOIN catalog.provider_credential credential ON credential.id=check_run.credential_id LEFT JOIN catalog.model_profile model ON model.id=check_run.model_id WHERE check_run.id=?`, id).Scan(&result)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrProviderNotFound
		}
		var rows []struct {
			ProviderModelID, DisplayName, Capability string
			Metadata                                 []byte
		}
		if err := tx.Raw(`SELECT provider_model_id,display_name,capability,metadata FROM catalog.model_candidate WHERE check_id=? ORDER BY capability,provider_model_id LIMIT 600`, id).Scan(&rows).Error; err != nil {
			return err
		}
		result.Candidates = []application.ModelCandidate{}
		for _, row := range rows {
			result.Candidates = append(result.Candidates, application.ModelCandidate{ProviderModelID: row.ProviderModelID, DisplayName: row.DisplayName, Capability: row.Capability, Metadata: json.RawMessage(row.Metadata)})
		}
		return nil
	})
	return result, err
}

// FailProviderCheck closes only the exact worker-owned request without exposing errors.
func (s *Store) FailProviderCheck(ctx context.Context, id, actor, provider, credential uuid.UUID) error {
	return taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		if err := tx.Exec(`UPDATE catalog.provider_check SET status='failed',error_code='check_interrupted',model_result=CASE WHEN kind='model_access' AND dispatch_started_at IS NOT NULL THEN 'unknown' ELSE model_result END,completed_at=now() WHERE id=? AND actor_id=? AND provider_id=? AND credential_id IS NOT DISTINCT FROM NULLIF(?::uuid,'00000000-0000-0000-0000-000000000000'::uuid) AND status IN ('pending','running')`, id, actor, provider, credential).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE catalog.provider_check original SET status='failed',model_result='unknown',error_code='check_interrupted',completed_at=now() FROM catalog.provider_check query WHERE query.id=? AND query.actor_id=? AND query.provider_id=? AND query.credential_id=? AND query.kind='model_query' AND query.parent_check_id=original.id AND original.status='running'`, id, actor, provider, credential).Error
	})
}

// ListProviderChecks returns safe evidence; historical passes never become current.
func (s *Store) ListProviderChecks(ctx context.Context, actor identityapp.Principal, id uuid.UUID, offset int) (application.ProviderCheckPage, error) {
	page := application.ProviderCheckPage{Items: []application.ProviderCheck{}}
	err := taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		if err := s.requirePlatformAdmin(tx, actor); err != nil {
			return err
		}
		var found int
		r := tx.Raw(`SELECT 1 FROM catalog.provider WHERE id=? AND NOT is_delete`, id).Scan(&found)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrProviderNotFound
		}
		err := tx.Raw(`SELECT check_run.*,check_run.create_time AS created_at,
 (p.current_config_version_id=check_run.config_version_id AND NOT p.is_delete AND
 (check_run.kind='reachability' OR (credential.status='active' AND NOT credential.is_delete)) AND
 (check_run.model_id IS NULL OR (model.current_version_id=check_run.model_version_id AND NOT model.is_delete))) AS current FROM catalog.provider_check check_run JOIN catalog.provider p ON p.id=check_run.provider_id
 LEFT JOIN catalog.provider_credential credential ON credential.id=check_run.credential_id LEFT JOIN catalog.model_profile model ON model.id=check_run.model_id
 WHERE check_run.provider_id=? ORDER BY check_run.create_time DESC,check_run.id DESC LIMIT 21 OFFSET ?`, id, offset).Scan(&page.Items).Error
		if err := tx.Raw(`SELECT DISTINCT ON (check_run.kind,check_run.model_id) check_run.*,check_run.create_time AS created_at,true AS current
 FROM catalog.provider_check check_run JOIN catalog.provider p ON p.id=check_run.provider_id AND NOT p.is_delete AND p.current_config_version_id=check_run.config_version_id
 LEFT JOIN catalog.provider_credential credential ON credential.id=check_run.credential_id LEFT JOIN catalog.model_profile model ON model.id=check_run.model_id
 WHERE check_run.provider_id=? AND (check_run.kind='reachability' OR (credential.status='active' AND NOT credential.is_delete))
 AND (check_run.model_id IS NULL OR (model.current_version_id=check_run.model_version_id AND NOT model.is_delete))
 ORDER BY check_run.kind,check_run.model_id,check_run.create_time DESC,check_run.id DESC LIMIT 70`, id).Scan(&page.CurrentChecks).Error; err != nil {
			return err
		}
		if len(page.Items) > 20 {
			page.HasMore = true
			page.Items = page.Items[:20]
		}
		return err
	})
	return page, err
}

// EndProviderCheckTracking keeps unknown evidence unchanged and touches no budget.
func (s *Store) EndProviderCheckTracking(ctx context.Context, actor identityapp.Principal, id, request uuid.UUID, note string) (application.ProviderCheck, error) {
	note = strings.TrimSpace(note)
	if note == "" || len(note) > 1000 {
		return application.ProviderCheck{}, application.ErrInvalidCredentialTest
	}
	return catalogMutation(ctx, s.db, s.actors, actor, "provider.check.end_tracking", id, request, map[string]string{"note": note}, func(tx *gorm.DB) (application.ProviderCheck, error) {
		r := tx.Exec(`UPDATE catalog.provider_check SET tracking_ended_at=COALESCE(tracking_ended_at,now()),tracking_note=? WHERE id=? AND kind='model_access' AND model_result='unknown' AND status NOT IN ('pending','running') AND task_id=''`, note, id)
		if r.Error != nil {
			return application.ProviderCheck{}, r.Error
		}
		if r.RowsAffected != 1 {
			return application.ProviderCheck{}, application.ErrInvalidCredentialTest
		}
		return s.withDB(tx).ReadProviderCheck(ctx, actor, id)
	})
}

// ListProviderTestModels keeps formal release trials separate from light onboarding.
func (s *Store) ListProviderTestModels(ctx context.Context, actor identityapp.Principal, id uuid.UUID) ([]application.ProviderTestModel, error) {
	models := []application.ProviderTestModel{}
	err := taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		if err := s.requirePlatformAdmin(tx, actor); err != nil {
			return err
		}
		return tx.Raw(`SELECT DISTINCT ON (m.id) m.id,m.current_version_id AS version_id,r.id AS release_id,m.display_name,v.provider_model_id,m.capability,m.status,
 (r.upstream_cost<>'{}'::jsonb OR m.candidate_snapshot IS NULL) AS requires_trial
 FROM catalog.model_profile m JOIN catalog.model_profile_version v ON v.id=m.current_version_id AND NOT v.is_delete
 JOIN catalog.model_release r ON r.model_id=m.id AND r.model_version_id=v.id AND r.status IN ('draft','active')
 JOIN catalog.provider p ON p.id=m.provider_id AND p.current_config_version_id=r.config_version_id AND NOT p.is_delete
 WHERE m.provider_id=? AND m.status<>'archived' AND NOT m.is_delete ORDER BY m.id,(r.id=m.active_release_id) DESC,r.create_time DESC LIMIT 64`, id).Scan(&models).Error
	})
	return models, err
}
