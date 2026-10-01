package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// FindProject reads a live project by both its ID and the actor's organization.
func (s *Store) FindProject(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID) (domain.Project, error) {
	if s == nil || s.db == nil {
		return domain.Project{}, ErrUnavailable
	}
	if projectID == uuid.Nil {
		return domain.Project{}, ErrProjectNotFound
	}
	var project domain.Project
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		var err error
		project, err = readProjectInTx(tx, actor.OrgID, projectID)
		return err
	})
	if err != nil {
		return domain.Project{}, fmt.Errorf("find project: %w", err)
	}
	return project, nil
}

func readProjectInTx(tx *gorm.DB, orgID, projectID uuid.UUID) (domain.Project, error) {
	var row struct {
		ID                  uuid.UUID
		OrgID               uuid.UUID
		Name                string
		Description         string
		AspectRatio         string
		StyleType           string
		StyleSubtype        *string
		StylePresetID       *uuid.UUID
		Resolution          string
		AllowOverseasModels bool
		Status              string
		ArchivedAt          *time.Time
		DeleteTime          *time.Time
		PurgeAfter          *time.Time
		Revision            int64
		CreateTime          time.Time
		UpdateTime          time.Time
	}
	result := tx.Raw(`
		SELECT id, org_id, name, description, aspect_ratio, style_type,
		       style_subtype, style_preset_id, resolution, allow_overseas_models,
		       status, archived_at, delete_time, purge_after, revision,
		       create_time, update_time
		FROM workspace.project
		WHERE id = ?::uuid AND org_id = ?::uuid AND NOT is_delete AND status IN ('active','archived')
	`, projectID.String(), orgID.String()).Scan(&row)
	if result.Error != nil {
		return domain.Project{}, fmt.Errorf("read project: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return domain.Project{}, ErrProjectNotFound
	}
	project := domain.Project{
		ID: row.ID, OrgID: row.OrgID, Name: row.Name, Description: row.Description,
		AspectRatio: row.AspectRatio, StyleType: row.StyleType, Resolution: row.Resolution,
		AllowOverseasModels: row.AllowOverseasModels, Status: row.Status,
		ArchivedAt: row.ArchivedAt, DeleteTime: row.DeleteTime,
		PurgeAfter: row.PurgeAfter, Revision: row.Revision,
		CreateTime: row.CreateTime, UpdateTime: row.UpdateTime,
	}
	if row.StyleSubtype != nil {
		project.StyleSubtype = *row.StyleSubtype
	}
	if row.StylePresetID != nil {
		project.StylePresetID = *row.StylePresetID
	}
	return project, nil
}
