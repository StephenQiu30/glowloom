package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
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
		var ids []uuid.UUID
		if err := tx.Model(&sourceRow{}).
			Where("org_id = ? AND project_id = ? AND media_asset_id IS NOT NULL", b.OrgID, b.SourceProjectID).
			Distinct("media_asset_id").Order("media_asset_id").Pluck("media_asset_id", &ids).Error; err != nil {
			return err
		}
		result = append(result, ids...)
		return nil
	})
	return result, err
}
