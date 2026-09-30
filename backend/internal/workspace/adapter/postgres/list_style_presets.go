package postgres

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

// ListStylePresetSummaries never selects private prompt or reference columns.
func (s *Store) ListStylePresetSummaries(ctx context.Context, actor identityapp.Principal, input application.ListStylePresetsInput) (application.StylePresetPage, error) {
	if s == nil || s.db == nil {
		return application.StylePresetPage{}, ErrUnavailable
	}
	if input.Limit < 1 || input.Limit > 200 {
		return application.StylePresetPage{}, application.ErrInvalidStylePresetList
	}
	var page application.StylePresetPage
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		query := `SELECT id,name,style_type,COALESCE(style_subtype,'') AS style_subtype
		 FROM workspace.style_preset WHERE org_id=? AND project_id IS NULL AND NOT is_delete`
		args := []any{actor.OrgID}
		if input.StyleType != "" {
			query += ` AND style_type=?`
			args = append(args, input.StyleType)
		}
		if input.StyleSubtype != "" {
			query += ` AND style_subtype=?`
			args = append(args, input.StyleSubtype)
		}
		if input.After != nil {
			query += ` AND (name,id)>(?,?::uuid)`
			args = append(args, input.After.Name, input.After.ID)
		}
		query += ` ORDER BY name,id LIMIT ?`
		args = append(args, input.Limit+1)
		if err := tx.Raw(query, args...).Scan(&page.Items).Error; err != nil {
			return fmt.Errorf("read safe preset metadata: %w", err)
		}
		if len(page.Items) > input.Limit {
			page.Items = page.Items[:input.Limit]
			last := page.Items[len(page.Items)-1]
			page.Next = &application.StylePresetCursor{ID: last.ID, Name: last.Name}
		}
		return nil
	})
	if err != nil {
		return application.StylePresetPage{}, fmt.Errorf("style preset list transaction: %w", err)
	}
	return page, nil
}

var _ application.StylePresetSummaryStore = (*Store)(nil)
