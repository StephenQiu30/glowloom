package postgres

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func recordProviderCheck(tx *gorm.DB, observation application.CredentialTestObservation) error {
	if observation.AuthResult == "" {
		return nil
	} // Existing legacy test workflows have no pinned check.
	allowed := []string{"ok", "unknown", "unsupported", "unreachable", "auth_failed", "insufficient_balance", "rate_limited", "invalid_parameters", "not_found", "upstream_unavailable", "partial", "invalid_response"}
	if !slices.Contains(allowed, observation.AuthResult) || !slices.Contains(allowed, observation.AdapterResult) || !slices.Contains(allowed, observation.DiscoveryResult) || len(observation.Candidates) > 600 {
		return application.ErrInvalidCredentialTest
	}
	status := "completed"
	if observation.AuthResult != "ok" {
		status = "failed"
	}
	r := tx.Exec(`UPDATE catalog.provider_check SET status=?,auth_result=?,adapter_result=?,discovery_result=?,completed_at=? WHERE id=? AND provider_id=? AND credential_id=?`, status, observation.AuthResult, observation.AdapterResult, observation.DiscoveryResult, observation.TestedAt, observation.TestID, observation.ProviderID, observation.CredentialID)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return application.ErrInvalidCredentialTest
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
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requirePlatformCatalogAdmin(tx, actor); err != nil {
			return err
		}
		r := tx.Raw(`SELECT id,provider_id,config_version_id,credential_id,kind,status,auth_result,adapter_result,discovery_result,error_code,create_time AS created_at,completed_at FROM catalog.provider_check WHERE id=?`, id).Scan(&result)
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
	return s.db.WithContext(ctx).Exec(`UPDATE catalog.provider_check SET status='failed',error_code='check_interrupted',completed_at=now() WHERE id=? AND actor_id=? AND provider_id=? AND credential_id=? AND status IN ('pending','running')`, id, actor, provider, credential).Error
}
