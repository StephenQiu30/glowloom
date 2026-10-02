package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
)

// ReferencedMedia freezes all historical document references before media admission.
// The caller's trusted freeze transaction retains the project locks; no body I/O
// or script snapshot is published by this private owning query.
func (s *ProjectCopyStore) ReferencedMedia(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding) ([]uuid.UUID, error) {
	result := make([]uuid.UUID, 0)
	err := s.ownTransaction(ctx, actor, b, true, func(tx *gorm.DB) error {
		if err := rejectPendingWrite(tx, b.OrgID, b.SourceProjectID); err != nil {
			return err
		}
		var rows []struct{ ID uuid.UUID }
		if err := tx.Raw(`SELECT DISTINCT media_asset_id AS id FROM script.script_source WHERE org_id=? AND project_id=? AND media_asset_id IS NOT NULL ORDER BY media_asset_id`, b.OrgID, b.SourceProjectID).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			result = append(result, row.ID)
		}
		return nil
	})
	return result, err
}
