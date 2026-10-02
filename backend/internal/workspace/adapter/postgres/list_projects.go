package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// ListProjectsForActor reads one organization-scoped keyset page after
// rechecking the actor and organization in the same transaction.
func (s *Store) ListProjectsForActor(ctx context.Context, actor identityapp.Principal, input application.ListProjectsInput) (application.ProjectListPage, error) {
	if s == nil || s.db == nil {
		return application.ProjectListPage{}, ErrUnavailable
	}
	if input.Limit < 1 || input.Limit > 200 ||
		(input.Status != "" && input.Status != "active" && input.Status != "archived") ||
		(input.After != nil && (input.After.ID == uuid.Nil || input.After.UpdateTime.IsZero())) ||
		(input.Deleted && input.FolderID != nil) {
		return application.ProjectListPage{}, application.ErrInvalidProjectList
	}
	var page application.ProjectListPage
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if input.FolderID != nil && *input.FolderID != uuid.Nil {
			var exists bool
			if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.project_folder WHERE id=? AND org_id=? AND actor_id=? AND NOT is_delete)`, *input.FolderID, actor.OrgID, actor.ID).Scan(&exists).Error; err != nil {
				return err
			}
			if !exists {
				return domain.ErrProjectFolderNotFound
			}
		}
		query := `
			SELECT p.id, p.org_id, p.name, p.aspect_ratio, p.style_type,
			       p.style_subtype, p.style_preset_id, p.cover_asset_id, p.resolution,
			       p.allow_overseas_models, p.status, p.is_delete,
			       p.archived_at, p.delete_time, p.purge_after, p.revision,
			       p.create_time, p.update_time, f.id AS folder_id,
			       coalesce(m.revision,0) AS placement_revision
			FROM workspace.project AS p
			LEFT JOIN workspace.project_folder_placement m ON m.project_id=p.id AND m.org_id=p.org_id AND m.actor_id=?::uuid
			LEFT JOIN workspace.project_folder f ON f.id=m.folder_id AND f.org_id=m.org_id AND f.actor_id=m.actor_id AND NOT f.is_delete
			WHERE p.org_id = ?::uuid AND p.is_delete = ? AND p.status IN ('active','archived')
		`
		args := []any{actor.ID.String(), actor.OrgID.String(), input.Deleted}
		if input.FolderID != nil {
			if *input.FolderID == uuid.Nil {
				query += ` AND f.id IS NULL`
			} else {
				query += ` AND f.id=?::uuid`
				args = append(args, input.FolderID.String())
			}
		}
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
			item := row.item()
			var err error
			item.CoverUnavailable, err = s.projectCoverUnavailable(ctx, tx, actor, domain.Project{ID: item.ID, CoverAssetID: item.CoverAssetID})
			if err != nil {
				return err
			}
			page.Projects = append(page.Projects, item)
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
	CoverAssetID        *uuid.UUID
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
	FolderID            *uuid.UUID
	PlacementRevision   int64
}

func (row projectListRow) item() application.ProjectListItem {
	item := application.ProjectListItem{CoverAssetID: row.CoverAssetID,
		ID: row.ID, OrgID: row.OrgID, Name: row.Name,
		AspectRatio: row.AspectRatio, StyleType: row.StyleType,
		StylePresetID: row.StylePresetID, Resolution: row.Resolution,
		AllowOverseasModels: row.AllowOverseasModels, Status: row.Status,
		IsDelete: row.IsDelete, ArchivedAt: row.ArchivedAt,
		DeleteTime: row.DeleteTime, PurgeAfter: row.PurgeAfter,
		Revision: row.Revision, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime,
		FolderID: row.FolderID, PlacementRevision: row.PlacementRevision,
	}
	if row.StyleSubtype != nil {
		item.StyleSubtype = *row.StyleSubtype
	}
	return item
}

var _ application.ListProjectsStore = (*Store)(nil)
