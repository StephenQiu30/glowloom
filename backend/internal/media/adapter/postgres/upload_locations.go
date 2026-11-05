package postgres

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type uploadLocationReservation struct {
	ID             uuid.UUID
	ProfileID      string
	Bucket         string
	ObjectKey      string
	OwnerSessionID uuid.UUID
}

func (uploadLocationReservation) TableName() string { return "media.object_location" }

// ReserveUploadLocation binds a unique object to its unexpired upload session.
func (s *StorageRoutes) ReserveUploadLocation(ctx context.Context, session uuid.UUID, ref domain.ObjectRef) (domain.ObjectLocation, error) {
	if s == nil || s.db == nil {
		return domain.ObjectLocation{}, application.ErrUnavailable
	}
	if session == uuid.Nil || ref.Validate() != nil {
		return domain.ObjectLocation{}, application.ErrInvalidUpload
	}
	var location domain.ObjectLocation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var admission uploadAdmissionRow
		err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("id = ? AND profile_id = ? AND bucket = ? AND expires_at > clock_timestamp() AND accepted_asset_id IS NULL", session, ref.ProfileID, ref.Bucket).Take(&admission).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return application.ErrUploadConflict
		}
		if err != nil {
			return err
		}
		candidate := uploadLocationReservation{ID: uuid.New(), ProfileID: ref.ProfileID, Bucket: ref.Bucket, ObjectKey: ref.Key, OwnerSessionID: session}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "profile_id"}, {Name: "bucket"}, {Name: "object_key"}}, DoNothing: true}).Create(&candidate).Error; err != nil {
			return err
		}
		var row objectLocationRow
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("profile_id = ? AND bucket = ? AND object_key = ?", ref.ProfileID, ref.Bucket, ref.Key).Take(&row).Error; err != nil {
			return err
		}
		if row.OwnerSessionID == nil || *row.OwnerSessionID != session || (row.State != domain.ObjectLocationReserved && row.State != domain.ObjectLocationPresent) {
			return application.ErrUploadConflict
		}
		location = row.domain()
		return nil
	})
	return location, err
}

// ConfirmUploadLocation saves checked metadata only for an unaccepted upload.
func (s *StorageRoutes) ConfirmUploadLocation(ctx context.Context, location domain.ObjectLocation, info application.ObjectInfo) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	if location.ID == uuid.Nil || location.Ref.Validate() != nil || info.Size < 1 || info.ContentType == "" || len(info.SHA256) != 64 {
		return application.ErrObjectMismatch
	}
	if _, err := hex.DecodeString(info.SHA256); err != nil {
		return application.ErrObjectMismatch
	}
	r := s.db.WithContext(ctx).Model(&objectLocationRow{}).
		Where("id = ? AND profile_id = ? AND bucket = ? AND object_key = ? AND owner_session_id IS NOT NULL AND state IN ?", location.ID, location.Ref.ProfileID, location.Ref.Bucket, location.Ref.Key, []domain.ObjectLocationState{domain.ObjectLocationReserved, domain.ObjectLocationPresent}).
		Where("(byte_size IS NULL OR byte_size = ?) AND (sha256 IS NULL OR sha256 = ?) AND (mime_type IS NULL OR mime_type = ?)", info.Size, info.SHA256, info.ContentType).
		Updates(map[string]any{"state": domain.ObjectLocationPresent, "byte_size": info.Size, "sha256": info.SHA256, "mime_type": info.ContentType})
	if r.Error != nil {
		return fmt.Errorf("confirm uploaded location: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return application.ErrObjectMismatch
	}
	return nil
}

// BeginUploadRemoval arbitrates compensation against acceptance; unknown I/O stays pending.
func (s *StorageRoutes) BeginUploadRemoval(ctx context.Context, location domain.ObjectLocation) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	if location.ID == uuid.Nil || location.Ref.Validate() != nil {
		return false, domain.ErrInvalidObjectRef
	}
	state := domain.ObjectLocationDeletePending
	if location.State == domain.ObjectLocationUnknown {
		state = domain.ObjectLocationUnknown
	}
	r := s.db.WithContext(ctx).Model(&objectLocationRow{}).Where("id = ? AND profile_id = ? AND bucket = ? AND object_key = ? AND owner_session_id IS NOT NULL AND state IN ?", location.ID, location.Ref.ProfileID, location.Ref.Bucket, location.Ref.Key, []domain.ObjectLocationState{domain.ObjectLocationReserved, domain.ObjectLocationPresent, domain.ObjectLocationDeletePending}).Update("state", state)
	if r.Error != nil {
		return false, r.Error
	}
	return r.RowsAffected == 1 && state != domain.ObjectLocationUnknown, nil
}

// ConfirmUploadRemoval records a known successful delete without erasing its identity.
func (s *StorageRoutes) ConfirmUploadRemoval(ctx context.Context, location domain.ObjectLocation) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	r := s.db.WithContext(ctx).Model(&objectLocationRow{}).Where("id = ? AND profile_id = ? AND bucket = ? AND object_key = ? AND owner_session_id IS NOT NULL AND state = ?", location.ID, location.Ref.ProfileID, location.Ref.Bucket, location.Ref.Key, domain.ObjectLocationDeletePending).Update("state", domain.ObjectLocationDeleted)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return application.ErrStorageRouteConflict
	}
	return nil
}

// acceptUploadLocations is part of the existing asset/receipt transaction. A
// cleanup worker and acceptance contend on the same location row; an accepted
// object cannot later be removed through upload compensation.
func acceptUploadLocations(tx *gorm.DB, actor identityapp.Principal, r application.UploadRequest, asset domain.MediaAsset, rends []domain.Rendition, accepted uuid.UUID) error {
	if r.Target == nil {
		if asset.ObjectLocation != nil {
			return application.ErrInvalidUpload
		}
		return nil
	}
	target := r.Target
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &r.ProjectID}
	if r.Personal != nil {
		scope = domain.LibraryScope{Kind: domain.LibraryPersonal}
	}
	scopeID, err := scope.Identity(actor.OrgID, actor.ID)
	if err != nil {
		return application.ErrUploadConflict
	}
	var admission uploadAdmissionRow
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND org_id = ? AND actor_id = ? AND scope_id = ? AND request_key = ? AND profile_id = ? AND bucket = ? AND config_revision = ? AND expires_at > clock_timestamp() AND accepted_asset_id IS NULL", target.SessionID, actor.OrgID, actor.ID, scopeID, r.Key, target.ProfileID, target.Bucket, target.ConfigRevision).Take(&admission).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && !admission.ExpiresAt.After(asset.CreateTime) {
		return application.ErrUploadConflict
	}
	if err != nil {
		return err
	}
	stored, err := admission.target()
	if err != nil || stored != *target {
		return application.ErrUploadConflict
	}
	if accepted == asset.ID {
		if asset.ObjectLocation == nil || len(rends) > 8 {
			return application.ErrInvalidUpload
		}
		locations := []*domain.ObjectLocation{asset.ObjectLocation}
		for _, rend := range rends {
			locations = append(locations, rend.ObjectLocation)
		}
		for _, location := range locations {
			if location == nil || location.Ref.ProfileID != target.ProfileID || location.Ref.Bucket != target.Bucket || location.State != domain.ObjectLocationPresent || location.Ref.Validate() != nil {
				return application.ErrInvalidUpload
			}
			update := tx.Model(&objectLocationRow{}).Where("id = ? AND owner_session_id = ? AND profile_id = ? AND bucket = ? AND object_key = ? AND state = ?", location.ID, target.SessionID, location.Ref.ProfileID, location.Ref.Bucket, location.Ref.Key, domain.ObjectLocationPresent).Update("state", domain.ObjectLocationAccepted)
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected != 1 {
				return application.ErrUploadConflict
			}
		}
	}
	update := tx.Model(&uploadAdmissionRow{}).Where("id = ? AND (accepted_asset_id IS NULL OR accepted_asset_id = ?)", target.SessionID, accepted).Update("accepted_asset_id", accepted)
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return application.ErrUploadConflict
	}
	return nil
}

// insertReviewedUpload establishes the physical identity in the first INSERT;
// runtime cannot rewrite accepted asset ownership and finalized columns are NOT NULL.
func insertReviewedUpload(tx *gorm.DB, a domain.MediaAsset, rends []domain.Rendition) error {
	row := map[string]any{"id": a.ID, "kind": string(a.Kind), "origin": string(domain.OriginUpload), "status": string(domain.StatusReady), "object_key": a.ObjectKey, "file_name": a.FileName, "mime_type": a.MimeType, "byte_size": a.ByteSize, "sha256": a.SHA256, "width": a.Width, "height": a.Height, "duration_ms": a.DurationMS, "fps": a.FPS, "audio_channels": a.AudioChannels, "codec": a.Codec, "moderation_status": string(domain.ModerationPassed), "moderation_detail": jsonDocument(a.ModerationDetail), "revision": 1, "create_time": a.CreateTime, "update_time": a.UpdateTime}
	if a.Personal != nil {
		row["personal_org_id"] = a.Personal.OrgID
		row["personal_actor_id"] = a.Personal.ActorID
	} else {
		row["project_id"] = a.ProjectID
	}
	if a.ObjectLocation != nil {
		row["object_location_id"] = a.ObjectLocation.ID
	}
	q := tx.Table("media.media_asset").Create(row)
	if q.Error != nil {
		return fmt.Errorf("insert reviewed upload: %w", q.Error)
	}
	if q.RowsAffected != 1 {
		return application.ErrUploadConflict
	}
	for _, r := range rends {
		row := map[string]any{"id": r.ID, "media_asset_id": r.MediaAssetID, "kind": string(r.Kind), "object_key": r.ObjectKey, "width": r.Width, "height": r.Height, "byte_size": r.ByteSize, "create_time": r.CreateTime, "update_time": r.UpdateTime}
		if r.ObjectLocation != nil {
			row["object_location_id"] = r.ObjectLocation.ID
		}
		q := tx.Table("media.rendition").Create(row)
		if q.Error != nil {
			return fmt.Errorf("insert upload rendition: %w", q.Error)
		}
		if q.RowsAffected != 1 {
			return application.ErrUploadConflict
		}
	}
	return nil
}
