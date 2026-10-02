package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// ReadConfiguredVoiceModels holds mutable availability rows until the caller
// transaction finishes. Published versions are INSERT-only for the runtime role;
// the model's locked current_version_id pins their immutable content.
func (s *Store) ReadConfiguredVoiceModels(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID, input application.VoiceModelInput) ([]application.ConfiguredVoiceModel, error) {
	if s == nil || s.db == nil {
		return nil, application.ErrVoiceCatalogUnavailable
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.MustChangePassword || actor.Role != identitydomain.RoleAdmin && actor.Role != identitydomain.RoleProducer {
		return nil, identityapp.ErrForbidden
	}
	if projectID == uuid.Nil || input.Limit < 1 || input.Limit > 200 || len(input.ModelKey) > 512 || input.After != nil && (strings.TrimSpace(input.After.Key) == "" || len(input.After.Key) > 512) {
		return nil, application.ErrInvalidVoiceSelection
	}
	models := []application.ConfiguredVoiceModel{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentModelCatalogActor(tx, actor); err != nil {
			return err
		}
		var project struct{ AllowOverseasModels bool }
		result := tx.Raw(`SELECT allow_overseas_models FROM workspace.project WHERE id=?::uuid AND org_id=?::uuid AND status='active' AND NOT is_delete FOR SHARE`, projectID.String(), actor.OrgID.String()).Scan(&project)
		if result.Error != nil {
			return fmt.Errorf("lock voice catalog project: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return application.ErrModelCatalogProjectNotFound
		}
		query := `SELECT m.model_key, m.display_name,
			v.id, v.model_profile_id, v.version_no, v.provider_model_id,
			to_jsonb(v.modes)::text AS modes, v.limits::text AS limits,
			v.param_schema::text AS param_schema, v.supports_query, v.supports_cancel,
			v.supports_callback, v.expected_max_ms, v.moderation, v.queue
			FROM catalog.model_profile AS m
			JOIN catalog.provider AS p ON p.id=m.provider_id
			JOIN catalog.capability AS cap ON cap.key=m.capability
			JOIN catalog.model_profile_version AS v ON v.id=m.current_version_id AND v.model_profile_id=m.id
			JOIN catalog.provider_credential AS c ON c.provider_id=p.id
			WHERE m.capability='audio.tts' AND m.status='active' AND NOT m.is_delete
			AND p.status='active' AND NOT p.is_delete AND (? OR p.region='domestic')
			AND NOT cap.is_delete AND NOT v.is_delete
			AND c.status='active' AND NOT c.is_delete AND c.last_test_result='ok'
			AND (?='' OR m.model_key=?)`
		args := []any{project.AllowOverseasModels, input.ModelKey, input.ModelKey}
		if input.After != nil {
			comparison := ">"
			if input.After.Inclusive {
				comparison = ">="
			}
			query += ` AND m.model_key COLLATE "C" ` + comparison + ` ? COLLATE "C"`
			args = append(args, input.After.Key)
		}
		query += ` ORDER BY m.model_key COLLATE "C" LIMIT ? FOR SHARE OF m,p,c`
		args = append(args, input.Limit)
		var rows []voiceModelRow
		if err := tx.Raw(query, args...).Scan(&rows).Error; err != nil {
			return fmt.Errorf("lock configured voice models: %w", err)
		}
		for _, row := range rows {
			var modes []string
			if err := json.Unmarshal([]byte(row.Modes), &modes); err != nil {
				return fmt.Errorf("decode voice model modes: %w", err)
			}
			models = append(models, application.ConfiguredVoiceModel{Key: row.ModelKey, DisplayName: row.DisplayName, Version: domain.ModelVersion{
				ID: row.ID, ModelID: row.ModelProfileID, VersionNo: row.VersionNo, ProviderModelID: row.ProviderModelID,
				Modes: modes, Limits: json.RawMessage(row.Limits), ParamSchema: json.RawMessage(row.ParamSchema),
				SupportsQuery: row.SupportsQuery, SupportsCancel: row.SupportsCancel, SupportsCallback: row.SupportsCallback,
				ExpectedMaxMS: row.ExpectedMaxMS, Moderation: domain.Moderation(row.Moderation), Queue: row.Queue,
			}})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read configured voice catalog: %w", err)
	}
	return models, nil
}

type voiceModelRow struct {
	ModelKey         string
	DisplayName      string
	ID               uuid.UUID
	ModelProfileID   uuid.UUID
	VersionNo        int
	ProviderModelID  string
	Modes            string
	Limits           string
	ParamSchema      string
	SupportsQuery    bool
	SupportsCancel   bool
	SupportsCallback bool
	ExpectedMaxMS    int
	Moderation       string
	Queue            string
}
