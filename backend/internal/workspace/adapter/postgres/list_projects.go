package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

// ListProjectsForActor reads one organization-scoped keyset page after
// rechecking the actor and organization in the same transaction.
func (s *Store) ListProjectsForActor(ctx context.Context, actor identityapp.Principal, input application.ListProjectsInput) (application.ProjectListPage, error) {
	if s == nil || s.db == nil {
		return application.ProjectListPage{}, ErrUnavailable
	}
	if input.Limit < 1 || input.Limit > 200 ||
		(input.Status != "" && input.Status != "active" && input.Status != "archived") ||
		(input.After != nil && (input.After.ID == uuid.Nil || input.After.UpdateTime.IsZero())) {
		return application.ProjectListPage{}, application.ErrInvalidProjectList
	}
	var page application.ProjectListPage
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		query := `
			SELECT p.id, p.org_id, p.name, p.aspect_ratio, p.style_type,
			       p.style_subtype, p.style_preset_id, p.resolution,
			       p.allow_overseas_models, p.status, p.is_delete,
			       p.archived_at, p.delete_time, p.purge_after, p.revision,
			       p.create_time, p.update_time
			FROM workspace.project AS p
			WHERE p.org_id = ?::uuid AND p.is_delete = ? AND p.status IN ('active','archived')
		`
		args := []any{actor.OrgID.String(), input.Deleted}
		if input.Status != "" {
			query += ` AND p.status = ?`
			args = append(args, input.Status)
		}
		if input.Query != "" {
			query += ` AND strpos(lower(p.name), lower(?)) > 0`
			args = append(args, input.Query)
		}
		if input.After != nil {
			query += ` AND (p.update_time, p.id) < (?::timestamptz, ?::uuid)`
			args = append(args, input.After.UpdateTime.UTC(), input.After.ID.String())
		}
		query += ` ORDER BY p.update_time DESC, p.id DESC LIMIT ?`
		args = append(args, input.Limit+1)

		var rows []projectListRow
		if err := tx.Raw(query, args...).Scan(&rows).Error; err != nil {
			return fmt.Errorf("read project list: %w", err)
		}
		more := len(rows) > input.Limit
		if more {
			rows = rows[:input.Limit]
		}
		page.Projects = make([]application.ProjectListItem, 0, len(rows))
		for _, row := range rows {
			page.Projects = append(page.Projects, row.item())
		}
		if more {
			last := page.Projects[len(page.Projects)-1]
			page.Next = &application.ProjectListCursor{UpdateTime: last.UpdateTime, ID: last.ID}
		}
		return nil
	})
	if err != nil {
		return application.ProjectListPage{}, fmt.Errorf("list projects transaction: %w", err)
	}
	return page, nil
}

type projectListRow struct {
	ID                  uuid.UUID
	OrgID               uuid.UUID
	Name                string
	AspectRatio         string
	StyleType           string
	StyleSubtype        *string
	StylePresetID       *uuid.UUID
	Resolution          string
	AllowOverseasModels bool
	Status              string
	IsDelete            bool
	ArchivedAt          *time.Time
	DeleteTime          *time.Time
	PurgeAfter          *time.Time
	Revision            int64
	CreateTime          time.Time
	UpdateTime          time.Time
}

func (row projectListRow) item() application.ProjectListItem {
	item := application.ProjectListItem{
		ID: row.ID, OrgID: row.OrgID, Name: row.Name,
		AspectRatio: row.AspectRatio, StyleType: row.StyleType,
		StylePresetID: row.StylePresetID, Resolution: row.Resolution,
		AllowOverseasModels: row.AllowOverseasModels, Status: row.Status,
		IsDelete: row.IsDelete, ArchivedAt: row.ArchivedAt,
		DeleteTime: row.DeleteTime, PurgeAfter: row.PurgeAfter,
		Revision: row.Revision, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime,
	}
	if row.StyleSubtype != nil {
		item.StyleSubtype = *row.StyleSubtype
	}
	return item
}

var _ application.ListProjectsStore = (*Store)(nil)
