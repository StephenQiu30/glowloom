package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// AuthorizeDerivedProject checks local media tool access within its transaction.
func (s *Store) AuthorizeDerivedProject(ctx context.Context, actor identityapp.Principal, project uuid.UUID, write bool) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		return s.requireProject(tx, actor, project, write)
	})
}

// StoreDerived records checked private bytes and real previews awaiting review.
func (s *Store) StoreDerived(ctx context.Context, actor identityapp.Principal, asset domain.MediaAsset, renditions []domain.Rendition) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := s.requireProject(tx, actor, asset.ProjectID, true); err != nil {
			return err
		}
		if err := asset.Validate(); err != nil {
			return err
		}
		if asset.Origin != domain.OriginSystem || asset.Status != domain.StatusProcessing || asset.ModerationStatus != domain.ModerationPending {
			return domain.ErrInvalidMediaAsset
		}
		row := map[string]any{"id": asset.ID, "project_id": asset.ProjectID, "kind": string(asset.Kind), "origin": "system", "status": "processing", "object_key": asset.ObjectKey, "file_name": asset.FileName, "mime_type": asset.MimeType, "byte_size": asset.ByteSize, "sha256": asset.SHA256, "width": asset.Width, "height": asset.Height, "duration_ms": asset.DurationMS, "fps": asset.FPS, "audio_channels": asset.AudioChannels, "codec": asset.Codec, "moderation_status": "pending", "moderation_detail": gorm.Expr("'{}'::jsonb"), "revision": 1, "create_time": asset.CreateTime, "update_time": asset.UpdateTime}
		if asset.ObjectLocation != nil {
			row["object_location_id"] = asset.ObjectLocation.ID
		}
		if err := tx.Table("media.media_asset").Create(row).Error; err != nil {
			return fmt.Errorf("store derived media: %w", err)
		}
		for _, r := range renditions {
			if r.Validate() != nil || r.MediaAssetID != asset.ID {
				return domain.ErrInvalidRendition
			}
			row := map[string]any{"id": r.ID, "media_asset_id": r.MediaAssetID, "kind": string(r.Kind), "object_key": r.ObjectKey, "width": r.Width, "height": r.Height, "byte_size": r.ByteSize, "create_time": r.CreateTime, "update_time": r.UpdateTime}
			if r.ObjectLocation != nil {
				row["object_location_id"] = r.ObjectLocation.ID
			}
			if err := tx.Table("media.rendition").Create(row).Error; err != nil {
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
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := s.requireProject(tx, actor, project, true); err != nil {
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
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := s.requireProject(tx, actor, project, true); err != nil {
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

// FinalizeDerived inherits reference eligibility only after checking every source
// in the publication transaction. No user inspection or consent is invented.
func (s *Store) FinalizeDerived(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID, sha string, sources []uuid.UUID, now time.Time) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := s.requireProject(tx, actor, project, true); err != nil {
			return err
		}
		scoped := *s
		scoped.db = tx
		for _, sourceID := range sources {
			source, err := scoped.FindAsset(ctx, actor, project, sourceID)
			if err != nil {
				return err
			}
			if !source.CanReference() || source.ContainsRealPerson || source.ConsentRecordID != nil {
				return domain.ErrInvalidMediaAsset
			}
		}
		detail, err := json.Marshal(struct {
			Method     string      `json:"method"`
			VerifiedAt time.Time   `json:"verified_at"`
			SHA256     string      `json:"sha256"`
			Sources    []uuid.UUID `json:"source_asset_ids"`
		}{"deterministic_timeline_source_validation", now, sha, sources})
		if err != nil {
			return err
		}
		result := tx.Exec(`UPDATE media.media_asset SET status='ready',moderation_status='passed',moderation_detail=?::jsonb,revision=revision+1,update_time=? WHERE id=? AND project_id=? AND origin='system' AND status='processing' AND moderation_status='pending' AND sha256=? AND NOT is_delete`, string(detail), now, id, project, sha)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrOutputConflict
		}
		return nil
	})
}
