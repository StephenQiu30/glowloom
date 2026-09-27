package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// ListStylePresets returns organization presets and, when requested, presets
// owned by one live project in that organization.
func (s *Store) ListStylePresets(ctx context.Context, actor identityapp.Principal, projectID *uuid.UUID) ([]domain.StylePreset, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if projectID != nil && *projectID == uuid.Nil {
		return nil, ErrProjectNotFound
	}
	var presets []domain.StylePreset
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		query := `
			SELECT id, org_id, project_id, name, style_type, style_subtype,
			       prompt_fragment, negative_prompt,
			       array_to_json(reference_asset_ids)::text AS reference_asset_ids_json
			FROM workspace.style_preset
			WHERE org_id = ?::uuid AND project_id IS NULL AND NOT is_delete
			ORDER BY name, id
		`
		args := []any{actor.OrgID.String()}
		if projectID != nil {
			var visible int
			result := tx.Raw(`
				SELECT 1 FROM workspace.project
				WHERE id = ?::uuid AND org_id = ?::uuid AND NOT is_delete
				FOR SHARE
			`, projectID.String(), actor.OrgID.String()).Scan(&visible)
			if result.Error != nil {
				return fmt.Errorf("check style preset project: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return ErrProjectNotFound
			}
			query = `
				SELECT id, org_id, project_id, name, style_type, style_subtype,
				       prompt_fragment, negative_prompt,
				       array_to_json(reference_asset_ids)::text AS reference_asset_ids_json
				FROM workspace.style_preset
				WHERE org_id = ?::uuid AND (project_id IS NULL OR project_id = ?::uuid)
				  AND NOT is_delete
				ORDER BY name, id
			`
			args = append(args, projectID.String())
		}
		var rows []struct {
			ID                    uuid.UUID
			OrgID                 uuid.UUID
			ProjectID             *uuid.UUID
			Name                  string
			StyleType             string
			StyleSubtype          *string
			PromptFragment        string
			NegativePrompt        string
			ReferenceAssetIDsJSON string
		}
		if err := tx.Raw(query, args...).Scan(&rows).Error; err != nil {
			return fmt.Errorf("list style presets: %w", err)
		}
		presets = make([]domain.StylePreset, 0, len(rows))
		for _, row := range rows {
			preset := domain.StylePreset{
				ID: row.ID, OrgID: row.OrgID, Name: row.Name, StyleType: row.StyleType,
				PromptFragment: row.PromptFragment, NegativePrompt: row.NegativePrompt,
			}
			if row.ProjectID != nil {
				preset.ProjectID = *row.ProjectID
			}
			if row.StyleSubtype != nil {
				preset.StyleSubtype = *row.StyleSubtype
			}
			if err := json.Unmarshal([]byte(row.ReferenceAssetIDsJSON), &preset.ReferenceAssetIDs); err != nil {
				return fmt.Errorf("decode style preset reference assets: %w", err)
			}
			presets = append(presets, preset)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read style presets: %w", err)
	}
	return presets, nil
}
