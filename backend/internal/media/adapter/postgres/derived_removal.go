package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
)

// AuthorizeDerivedRemoval checks real metadata ownership without exposing keys.
func (s *Store) AuthorizeDerivedRemoval(ctx context.Context, actor identityapp.Principal, in application.DerivedRemoval) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := requireProject(tx, actor, in.ProjectID, true); err != nil {
			return err
		}
		var rows []struct {
			ID, ProjectID                                  uuid.UUID
			Origin, Status, ModerationStatus, SHA256, Kind string
			ByteSize                                       int64
		}
		if err := tx.Raw(`SELECT id,project_id,origin,status,moderation_status,sha256,'original' AS kind,byte_size FROM media.media_asset WHERE object_key=? UNION ALL SELECT a.id,a.project_id,a.origin,a.status,a.moderation_status,a.sha256,r.kind,r.byte_size FROM media.rendition r JOIN media.media_asset a ON a.id=r.media_asset_id WHERE r.object_key=?`, in.ObjectKey, in.ObjectKey).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) > 1 {
			return ErrOutputConflict
		}
		for _, r := range rows {
			if r.ID != in.AssetID || r.ProjectID != in.ProjectID || r.Origin != "system" || r.Status != "processing" || r.ModerationStatus != "pending" || r.SHA256 != in.PrimarySHA256 || r.Kind != in.Kind || r.ByteSize != in.ByteSize {
				return ErrOutputConflict
			}
		}
		return nil
	})
}
