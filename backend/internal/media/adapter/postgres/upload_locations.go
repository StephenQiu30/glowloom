package postgres

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

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
		if err := tx.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,owner_session_id) SELECT ?,profile_id,bucket,?,id FROM media.upload_admission WHERE id=? AND profile_id=? AND bucket=? AND expires_at>now() AND accepted_asset_id IS NULL ON CONFLICT(profile_id,bucket,object_key) DO NOTHING`, uuid.New(), ref.Key, session, ref.ProfileID, ref.Bucket).Error; err != nil {
			return err
		}
		var row objectLocationRow
		read := tx.Raw(`SELECT * FROM media.object_location WHERE profile_id=? AND bucket=? AND object_key=? FOR SHARE`, ref.ProfileID, ref.Bucket, ref.Key).Scan(&row)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 || row.OwnerSessionID == nil || *row.OwnerSessionID != session || (row.State != "reserved" && row.State != "present") {
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
	r := s.db.WithContext(ctx).Exec(`UPDATE media.object_location SET state='present',byte_size=?,sha256=?,mime_type=? WHERE id=? AND profile_id=? AND bucket=? AND object_key=? AND owner_session_id IS NOT NULL AND state IN ('reserved','present') AND (byte_size IS NULL OR byte_size=?) AND (sha256 IS NULL OR sha256=?) AND (mime_type IS NULL OR mime_type=?)`, info.Size, info.SHA256, info.ContentType, location.ID, location.Ref.ProfileID, location.Ref.Bucket, location.Ref.Key, info.Size, info.SHA256, info.ContentType)
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
	state := "delete_pending"
	if location.State == "unknown" {
		state = "unknown"
	}
	r := s.db.WithContext(ctx).Exec(`UPDATE media.object_location SET state=? WHERE id=? AND profile_id=? AND bucket=? AND object_key=? AND owner_session_id IS NOT NULL AND state IN ('reserved','present','delete_pending')`, state, location.ID, location.Ref.ProfileID, location.Ref.Bucket, location.Ref.Key)
	if r.Error != nil {
		return false, r.Error
	}
	return r.RowsAffected == 1 && state != "unknown", nil
}

// ConfirmUploadRemoval records a known successful delete without erasing its identity.
func (s *StorageRoutes) ConfirmUploadRemoval(ctx context.Context, location domain.ObjectLocation) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	r := s.db.WithContext(ctx).Exec(`UPDATE media.object_location SET state='deleted' WHERE id=? AND profile_id=? AND bucket=? AND object_key=? AND owner_session_id IS NOT NULL AND state='delete_pending'`, location.ID, location.Ref.ProfileID, location.Ref.Bucket, location.Ref.Key)
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
	read := tx.Raw(`SELECT * FROM media.upload_admission WHERE id=? AND org_id=? AND actor_id=? AND scope_id=? AND request_key=? AND profile_id=? AND bucket=? AND config_revision=? AND expires_at>now() FOR UPDATE`, target.SessionID, actor.OrgID, actor.ID, scopeID, r.Key, target.ProfileID, target.Bucket, target.ConfigRevision).Scan(&admission)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 || !admission.ExpiresAt.After(asset.CreateTime) {
		return application.ErrUploadConflict
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
			if location == nil || location.Ref.ProfileID != target.ProfileID || location.Ref.Bucket != target.Bucket || location.State != "present" || location.Ref.Validate() != nil {
				return application.ErrInvalidUpload
			}
			update := tx.Exec(`UPDATE media.object_location SET state='accepted' WHERE id=? AND owner_session_id=? AND profile_id=? AND bucket=? AND object_key=? AND state='present'`, location.ID, target.SessionID, location.Ref.ProfileID, location.Ref.Bucket, location.Ref.Key)
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected != 1 {
				return application.ErrUploadConflict
			}
		}
		if err := tx.Exec(`UPDATE media.media_asset SET object_location_id=? WHERE id=? AND object_key=?`, asset.ObjectLocation.ID, asset.ID, asset.ObjectKey).Error; err != nil {
			return err
		}
		for _, rend := range rends {
			if err := tx.Exec(`UPDATE media.rendition SET object_location_id=? WHERE id=? AND object_key=?`, rend.ObjectLocation.ID, rend.ID, rend.ObjectKey).Error; err != nil {
				return err
			}
		}
	}
	update := tx.Exec(`UPDATE media.upload_admission SET accepted_asset_id=? WHERE id=? AND (accepted_asset_id IS NULL OR accepted_asset_id=?)`, accepted, target.SessionID, accepted)
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return application.ErrUploadConflict
	}
	return nil
}
