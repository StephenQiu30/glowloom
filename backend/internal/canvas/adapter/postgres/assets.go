package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

// AssetAccessFactory binds the inventory authority to the read transaction.
type AssetAccessFactory func(*gorm.DB) application.AssetAccess

// AssetStore owns the actual-document asset inventory.
type AssetStore struct {
	db     *gorm.DB
	access AssetAccessFactory
}

// NewAssetStore injects the existing database and current request authority.
func NewAssetStore(database *gorm.DB, access AssetAccessFactory) *AssetStore {
	return &AssetStore{db: database, access: access}
}

// ListAssets authorizes the selected scope even when the result is empty.
func (s *AssetStore) ListAssets(ctx context.Context, actor identity.Principal, input application.AssetListInput) (application.AssetPage, error) {
	page := application.AssetPage{Items: []application.AssetCanvas{}}
	input, err := application.NormalizeAssetListInput(input)
	if err != nil {
		return page, err
	}
	if s == nil || s.db == nil || s.access == nil || actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.ScopeID == uuid.Nil {
		return page, application.ErrNotFound
	}
	err = db.ReadSnapshot(ctx, s.db, func(tx *gorm.DB) error {
		page = application.AssetPage{Items: []application.AssetCanvas{}}
		access := s.access(tx)
		if access == nil {
			return application.ErrNotFound
		}
		if err := access.AuthorizeScope(ctx, actor); err != nil {
			return err
		}
		column := "d.update_time"
		if input.Sort == "created" {
			column = "d.create_time"
		}
		query := `SELECT d.id,d.project_id,
 CASE WHEN p.initial_canvas_id=d.id THEN p.name ELSE d.name END AS name,
 d.kind,d.revision,d.create_time AS created_at,d.update_time AS updated_at,
 d.asset_collection AS collection,COALESCE(pref.favorite,false) AS favorite,
 COALESCE(pref.revision,0) AS favorite_revision,p.revision AS project_revision,
 (d.asset_collection='current') AS can_copy_project,
 CASE WHEN p.initial_canvas_id=d.id AND p.cover_asset_id IS NOT NULL THEN p.cover_asset_id
 ELSE (SELECT n.ref_id FROM canvas.node n WHERE n.document_id=d.id AND n.node_type='image'
 AND n.ref_type='media_asset' AND n.ref_id IS NOT NULL AND NOT n.is_delete ORDER BY n.z_index,n.id LIMIT 1) END AS cover_asset_id,
 COALESCE(p.initial_canvas_id=d.id,false) AS is_main
 FROM canvas.document d JOIN workspace.project p ON p.id=d.project_id
 LEFT JOIN canvas.asset_preference pref ON pref.document_id=d.id AND pref.actor_id=?::uuid AND pref.scope_id=?::uuid
 WHERE p.org_id=?::uuid AND p.owner_scope_id=?::uuid AND NOT p.is_delete
 AND p.status IN ('active','archived') AND NOT d.is_delete AND d.kind=? AND d.asset_collection=?`
		args := []any{actor.ID, actor.ScopeID, actor.OrgID, actor.ScopeID, input.Kind, input.Collection}
		if input.FavoriteOnly {
			query += ` AND COALESCE(pref.favorite,false)`
		}

		if input.Search != "" {
			query += ` AND strpos(lower(CASE WHEN p.initial_canvas_id=d.id THEN p.name ELSE d.name END),lower(?))>0`
			args = append(args, input.Search)
		}
		if input.CreatedFrom != nil {
			query += ` AND d.create_time>=?`
			args = append(args, *input.CreatedFrom)
		}
		if input.CreatedBefore != nil {
			query += ` AND d.create_time<?`
			args = append(args, *input.CreatedBefore)
		}
		direction, comparison := "DESC", "<"
		if input.Order == "asc" {
			direction, comparison = "ASC", ">"
		}
		if input.After != nil {
			query += ` AND (` + column + `,d.id)` + comparison + `(?,?::uuid)`
			args = append(args, input.After.Time, input.After.ID)
		}
		query += ` ORDER BY ` + column + ` ` + direction + `,d.id ` + direction + ` LIMIT ?`
		args = append(args, input.Limit+1)
		var items []application.AssetCanvas
		if err := tx.Raw(query, args...).Scan(&items).Error; err != nil {
			return fmt.Errorf("read canvas assets: %w", err)
		}
		checked := make(map[uuid.UUID]bool)
		for _, item := range items {
			if !checked[item.ProjectID] {
				if err := access.AuthorizeProject(ctx, actor, item.ProjectID); err != nil {
					return err
				}
				checked[item.ProjectID] = true
			}
		}
		for index := range items {
			if items[index].CoverAssetID != nil {
				available, err := access.AssetCoverAvailable(ctx, actor, items[index].ProjectID, *items[index].CoverAssetID)
				if err != nil {
					return err
				}
				if !available {
					items[index].CoverAssetID = nil
					items[index].CoverUnavailable = true
				}
			}
		}
		if len(items) > input.Limit {
			items = items[:input.Limit]
			last := items[len(items)-1]
			position := last.UpdatedAt
			if input.Sort == "created" {
				position = last.CreatedAt
			}
			page.Next = &application.AssetCursor{ID: last.ID, Time: position}
		}
		page.Items = append(page.Items, items...)
		return nil
	})
	return page, err
}
