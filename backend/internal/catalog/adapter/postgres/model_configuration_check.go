package postgres

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/paramvalidation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// CheckModelConfiguration checks the current saved model and connection without
// dispatching an operation or requiring a price or release.
func (s *Store) CheckModelConfiguration(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (application.ModelConfigurationCheck, error) {
	var result application.ModelConfigurationCheck
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requirePlatformAdmin(tx, actor); err != nil {
			return err
		}
		var row struct {
			ID, ModelID                                     uuid.UUID
			VersionNo                                       int
			ProviderModelID                                 string
			Limits, ParamSchema                             json.RawMessage
			ExpectedMaxMS                                   int
			Moderation                                      domain.Moderation
			Queue                                           string
			SupportsQuery, SupportsCancel, SupportsCallback bool
			ProviderID, ConfigVersionID                     uuid.UUID
			CredentialID                                    *uuid.UUID
			Capability                                      string
			ModesJSON, ConfigJSON                           []byte
		}
		r := tx.Raw(`SELECT v.id,v.model_profile_id AS model_id,v.version_no,v.provider_model_id,
			to_jsonb(v.modes) AS modes_json,v.limits,v.param_schema,v.expected_max_ms,v.moderation,v.queue,
			v.supports_query,v.supports_cancel,v.supports_callback,
			p.id AS provider_id,c.id AS config_version_id,c.config AS config_json,k.id AS credential_id,m.capability
			FROM catalog.model_profile m
			JOIN catalog.model_profile_version v ON v.id=m.current_version_id AND NOT v.is_delete
			JOIN catalog.provider p ON p.id=m.provider_id AND NOT p.is_delete
			JOIN catalog.provider_config_version c ON c.id=p.current_config_version_id
			LEFT JOIN catalog.provider_credential k ON k.provider_id=p.id AND k.status='active' AND NOT k.is_delete
			WHERE m.id=? AND NOT m.is_delete`, id).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrModelSourceUnavailable
		}
		result = application.ModelConfigurationCheck{
			ProviderID: row.ProviderID, ConfigVersionID: row.ConfigVersionID,
			ModelVersionID: row.ID, CredentialID: row.CredentialID, ProviderModelID: row.ProviderModelID,
		}
		var config domain.ProviderConfig
		if err := json.Unmarshal(row.ConfigJSON, &config); err != nil {
			return err
		}
		var modes []string
		if err := json.Unmarshal(row.ModesJSON, &modes); err != nil {
			return err
		}
		version := domain.ModelVersion{ID: row.ID, ModelID: row.ModelID, VersionNo: row.VersionNo, ProviderModelID: row.ProviderModelID, Modes: modes, Limits: row.Limits, ParamSchema: row.ParamSchema, ExpectedMaxMS: row.ExpectedMaxMS, Moderation: row.Moderation, Queue: row.Queue, SupportsQuery: row.SupportsQuery, SupportsCancel: row.SupportsCancel, SupportsCallback: row.SupportsCallback}
		validator, err := paramvalidation.NewValidator()
		if err != nil {
			return err
		}
		if config.Validate() != nil ||
			!slices.Contains(config.Capabilities, strings.TrimSuffix(row.Capability, ".generate")) ||
			validator.Validate(version) != nil {
			return nil
		}
		var fields []struct {
			Field string `json:"field"`
		}
		if err := json.Unmarshal(row.ParamSchema, &fields); err != nil {
			return err
		}
		allowed := config.ParameterFields(row.Capability)
		for _, field := range fields {
			if !slices.Contains(allowed, field.Field) {
				return nil
			}
		}
		result.ParametersValid = true
		return nil
	})
	return result, err
}
