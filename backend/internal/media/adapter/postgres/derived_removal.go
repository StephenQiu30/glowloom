package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type derivedObjectRecord struct {
	ID               uuid.UUID
	ProjectID        uuid.UUID
	Origin           domain.Origin
	Status           domain.Status
	ModerationStatus domain.ModerationStatus
	SHA256           *string
	Kind             string
	ByteSize         int64
}

// AuthorizeDerivedRemoval checks real metadata ownership without exposing keys.
func (s *Store) AuthorizeDerivedRemoval(ctx context.Context, actor identityapp.Principal, in application.DerivedRemoval) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := s.requireProject(tx, actor, in.ProjectID, true); err != nil {
			return err
		}
		var rows []derivedObjectRecord
		if err := tx.Model(&assetRow{}).Select("id, project_id, origin, status, moderation_status, sha256, ? AS kind, byte_size", "original").Where("object_key = ?", in.ObjectKey).Limit(2).Scan(&rows).Error; err != nil {
			return err
		}
		var renditionRows []derivedObjectRecord
		if err := tx.Table("media.rendition AS r").Select("a.id, a.project_id, a.origin, a.status, a.moderation_status, a.sha256, r.kind, r.byte_size").Joins("JOIN media.media_asset AS a ON a.id = r.media_asset_id").Where("r.object_key = ?", in.ObjectKey).Limit(2).Scan(&renditionRows).Error; err != nil {
			return err
		}
		rows = append(rows, renditionRows...)
		if len(rows) > 1 {
			return ErrOutputConflict
		}
		for _, r := range rows {
			if r.ID != in.AssetID || r.ProjectID != in.ProjectID || r.Origin != domain.OriginSystem || r.Status != domain.StatusProcessing || r.ModerationStatus != domain.ModerationPending || r.SHA256 == nil || *r.SHA256 != in.PrimarySHA256 || r.Kind != in.Kind || r.ByteSize != in.ByteSize {
				return ErrOutputConflict
			}
		}
		return nil
	})
}
