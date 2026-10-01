package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// AuthorizeDerivedProject checks local media tool access within its transaction.
func (s *Store) AuthorizeDerivedProject(ctx context.Context, actor identityapp.Principal, project uuid.UUID, write bool) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		return requireProject(tx, actor, project, write)
	})
}

// StoreDerived records checked private bytes and real previews awaiting review.
func (s *Store) StoreDerived(ctx context.Context, actor identityapp.Principal, asset domain.MediaAsset, renditions []domain.Rendition) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := requireProject(tx, actor, asset.ProjectID, true); err != nil {
			return err
		}
		if err := asset.Validate(); err != nil {
			return err
		}
		if asset.Origin != domain.OriginSystem || asset.Status != domain.StatusProcessing || asset.ModerationStatus != domain.ModerationPending {
			return domain.ErrInvalidMediaAsset
		}
		if err := tx.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,width,height,duration_ms,fps,audio_channels,codec,moderation_status,moderation_detail,revision,create_time,update_time) VALUES(?,?,?,'system','processing',?,?,?,?,?,?,?,?,?,?,?,'pending','{}'::jsonb,1,?,?)`, asset.ID, asset.ProjectID, asset.Kind, asset.ObjectKey, asset.FileName, asset.MimeType, asset.ByteSize, asset.SHA256, asset.Width, asset.Height, asset.DurationMS, asset.FPS, asset.AudioChannels, asset.Codec, asset.CreateTime, asset.UpdateTime).Error; err != nil {
			return fmt.Errorf("store derived media: %w", err)
		}
		for _, r := range renditions {
			if r.Validate() != nil || r.MediaAssetID != asset.ID {
				return domain.ErrInvalidRendition
			}
			if err := tx.Exec(`INSERT INTO media.rendition(id,media_asset_id,kind,object_key,width,height,byte_size,create_time,update_time) VALUES(?,?,?,?,?,?,?,?,?)`, r.ID, r.MediaAssetID, r.Kind, r.ObjectKey, r.Width, r.Height, r.ByteSize, r.CreateTime, r.UpdateTime).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ReviewDerived accepts only the current produced bytes after human inspection.
func (s *Store) ReviewDerived(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID, sha string, now time.Time) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := requireProject(tx, actor, project, true); err != nil {
			return err
		}
		detail, err := json.Marshal(struct {
			Method                            string    `json:"method"`
			PrincipalID                       uuid.UUID `json:"principal_id"`
			ReviewedAt                        time.Time `json:"reviewed_at"`
			SHA256                            string    `json:"sha256"`
			RightsConfirmed                   bool      `json:"rights_confirmed"`
			NoAuthorizationRequiredRealPerson bool      `json:"no_authorization_required_real_person"`
		}{"local_workspace_owner_output_review", actor.ID, now, sha, true, true})
		if err != nil {
			return err
		}
		r := tx.Exec(`UPDATE media.media_asset SET status='ready',moderation_status='passed',moderation_detail=?::jsonb,revision=revision+1,update_time=? WHERE id=? AND project_id=? AND origin='system' AND status='processing' AND moderation_status='pending' AND sha256=? AND NOT is_delete`, string(detail), now, id, project, sha)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrOutputConflict
		}
		return nil
	})
}

// RejectDerived makes a cancelled, unreviewed artifact permanently unavailable.
func (s *Store) RejectDerived(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID, reason string, now time.Time) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := requireProject(tx, actor, project, true); err != nil {
			return err
		}
		r := tx.Exec(`UPDATE media.media_asset SET status='rejected',moderation_status='rejected',failure_reason=?,revision=revision+1,update_time=? WHERE id=? AND project_id=? AND origin='system' AND status='processing' AND moderation_status='pending' AND NOT is_delete`, reason, now, id, project)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrOutputConflict
		}
		return nil
	})
}
