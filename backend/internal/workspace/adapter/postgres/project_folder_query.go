package postgres

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// ListFolders reads bounded directory facts and reauthorizes every cover.
func (s *FolderStore) ListFolders(ctx context.Context, actor identityapp.Principal, in application.FolderListInput) (application.FolderListPage, error) {
	if s == nil || s.db == nil {
		return application.FolderListPage{}, application.ErrProjectDependencyUnavailable
	}
	if in.Limit < 1 || in.Limit > 200 {
		return application.FolderListPage{}, domain.ErrInvalidProjectFolder
	}
	var out application.FolderListPage
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		query := `SELECT f.id,f.org_id,f.actor_id,f.name,f.cover_project_id,f.cover_asset_id,f.revision,f.is_delete,f.delete_time,f.create_time,f.update_time,(SELECT count(*) FROM workspace.project_folder_placement m JOIN workspace.project p ON p.id=m.project_id AND p.org_id=m.org_id WHERE m.folder_id=f.id AND m.actor_id=f.actor_id AND m.org_id=f.org_id AND NOT p.is_delete AND p.status IN ('active','archived')) AS project_count FROM workspace.project_folder f WHERE f.org_id=? AND f.actor_id=? AND NOT f.is_delete`
		args := []any{actor.OrgID, actor.ID}
		if in.After != nil {
			query += ` AND (f.update_time,f.id)<(?,?)`
			args = append(args, in.After.UpdateTime.UTC(), in.After.ID)
		}
		query += ` ORDER BY f.update_time DESC,f.id DESC LIMIT ?`
		args = append(args, in.Limit+1)
		var rows []struct {
			Folder       folderRow `gorm:"embedded"`
			ProjectCount int64
		}
		if err := tx.Raw(query, args...).Scan(&rows).Error; err != nil {
			return fmt.Errorf("list personal directories: %w", err)
		}
		more := len(rows) > in.Limit
		if more {
			rows = rows[:in.Limit]
		}
		for _, row := range rows {
			folder, err := row.Folder.folder()
			if err != nil {
				return err
			}
			item := application.FolderSummary{Folder: folder, ProjectCount: row.ProjectCount}
			if folder.Cover != nil {
				if err := s.requireCover(ctx, tx, actor, folder.Cover); err != nil {
					if !errors.Is(err, mediaapp.ErrNotFound) && !errors.Is(err, domain.ErrProjectFolderCoverUnavailable) {
						return err
					}
					item.CoverUnavailable = true
					item.Folder.Cover = nil
				}
			}
			out.Items = append(out.Items, item)
		}
		if more {
			last := out.Items[len(out.Items)-1].Folder
			out.Next = &application.ProjectListCursor{ID: last.ID, UpdateTime: last.UpdateTime}
		}
		return nil
	})
	return out, err
}
