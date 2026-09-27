package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// CreateProject writes a new project and its zero budget in one transaction.
// The command layer adds audit and change events when the public route is wired.
func (s *Store) CreateProject(ctx context.Context, actor identityapp.Principal, project domain.Project) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if err := project.Validate(); err != nil {
		return err
	}
	if project.OrgID != actor.OrgID || project.Status != "active" || project.Revision != 1 ||
		project.AllowOverseasModels || project.IsDelete || project.ArchivedAt != nil ||
		project.DeleteTime != nil || project.PurgeAfter != nil {
		return domain.ErrInvalidProject
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := requireUsablePreset(tx, project); err != nil {
			return err
		}
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
	})
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
