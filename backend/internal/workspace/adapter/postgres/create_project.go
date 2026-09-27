package postgres

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func createProjectRowsInTx(tx *gorm.DB, project domain.Project) error {
	var subtype, presetID any
	if project.StyleSubtype != "" {
		subtype = project.StyleSubtype
	}
	if project.StylePresetID != uuid.Nil {
		presetID = project.StylePresetID.String()
	}
	inserted := tx.Exec(`
			INSERT INTO workspace.project
			  (id, org_id, name, description, aspect_ratio, style_type,
			   style_subtype, style_preset_id, resolution)
			VALUES (?::uuid, ?::uuid, ?, ?, ?, ?, ?, ?::uuid, ?)
		`, project.ID.String(), project.OrgID.String(), project.Name, project.Description,
		project.AspectRatio, project.StyleType, subtype, presetID, project.Resolution)
	if inserted.Error != nil {
		return fmt.Errorf("insert project: %w", inserted.Error)
	}
	if inserted.RowsAffected != 1 {
		return fmt.Errorf("insert project: inserted %d rows", inserted.RowsAffected)
	}
	budget := tx.Exec(`
			INSERT INTO billing.budget (id, project_id, limit_micros)
			VALUES (?::uuid, ?::uuid, 0)
		`, uuid.NewString(), project.ID.String())
	if budget.Error != nil {
		return fmt.Errorf("insert zero project budget: %w", budget.Error)
	}
	if budget.RowsAffected != 1 {
		return fmt.Errorf("insert zero project budget: inserted %d rows", budget.RowsAffected)
	}
	return nil
}

func requireUsablePreset(tx *gorm.DB, project domain.Project) error {
	if project.StylePresetID == uuid.Nil {
		return nil
	}
	var row struct {
		StyleType    string
		StyleSubtype *string
	}
	result := tx.Raw(`
		SELECT style_type, style_subtype
		FROM workspace.style_preset
		WHERE id = ?::uuid AND org_id = ?::uuid AND NOT is_delete
		  AND (project_id IS NULL OR project_id = ?::uuid)
		FOR SHARE
	`, project.StylePresetID.String(), project.OrgID.String(), project.ID.String()).Scan(&row)
	if result.Error != nil {
		return fmt.Errorf("check project style preset: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrStylePresetNotFound
	}
	subtype := ""
	if row.StyleSubtype != nil {
		subtype = *row.StyleSubtype
	}
	if row.StyleType != project.StyleType || subtype != project.StyleSubtype {
		return ErrStylePresetMismatch
	}
	return nil
}
