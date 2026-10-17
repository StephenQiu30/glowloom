package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type storageWorkRow struct {
	ID, OwnerID, WorkID                            uuid.UUID
	Kind, ProfileID, Bucket, ConfigRevision, State string
}

func (r storageWorkRow) intent() application.StorageWorkIntent {
	return application.StorageWorkIntent{ID: r.ID, Owner: application.StorageWorkOwner{Kind: r.Kind, OwnerID: r.OwnerID, WorkID: r.WorkID}, ProfileID: r.ProfileID, Bucket: r.Bucket, ConfigRevision: r.ConfigRevision, State: r.State}
}

func readStorageWork(tx *gorm.DB, owner application.StorageWorkOwner) (application.StorageWorkIntent, bool, error) {
	var row storageWorkRow
	read := tx.Raw(`SELECT * FROM media.storage_work_intent WHERE kind=? AND owner_id=? AND work_id=? FOR SHARE`, owner.Kind, owner.OwnerID, owner.WorkID).Scan(&row)
	return row.intent(), read.RowsAffected == 1, read.Error
}

// PinStorageWork is part of the caller's authorized admission transaction. A
// retry returns the original intent even if activation has since changed route.
func (s *StorageRoutes) PinStorageWork(ctx context.Context, owner application.StorageWorkOwner) (application.StorageWorkIntent, error) {
	var intent application.StorageWorkIntent
	if s == nil || s.db == nil {
		return intent, application.ErrUnavailable
	}
	if err := owner.Validate(); err != nil {
		return intent, err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		prior, found, err := readStorageWork(tx, owner)
		if err != nil {
			return err
		}
		if found {
			intent = prior
			return nil
		}
		route, err := readStorageRoute(tx, true)
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO media.storage_work_intent(id,kind,owner_id,work_id,profile_id,bucket,config_revision) VALUES(?,?,?,?,?,?,?) ON CONFLICT(kind,owner_id,work_id) DO NOTHING`, uuid.New(), owner.Kind, owner.OwnerID, owner.WorkID, route.ProfileID, route.Bucket, route.Revision).Error; err != nil {
			return err
		}
		intent, found, err = readStorageWork(tx, owner)
		if err != nil {
			return err
		}
		if !found {
			return application.ErrUnavailable
		}
		return nil
	})
	return intent, err
}

// ReadStorageWork never creates a new target during execution or recovery.
func (s *StorageRoutes) ReadStorageWork(ctx context.Context, owner application.StorageWorkOwner) (application.StorageWorkIntent, error) {
	if s == nil || s.db == nil {
		return application.StorageWorkIntent{}, application.ErrUnavailable
	}
	if err := owner.Validate(); err != nil {
		return application.StorageWorkIntent{}, err
	}
	intent, found, err := readStorageWork(s.db.WithContext(ctx), owner)
	if err != nil {
		return intent, err
	}
	if !found {
		return intent, application.ErrStorageNotReady
	}
	return intent, nil
}

// ReserveWorkLocation records ownership on the frozen target before SDK writes.
func (s *StorageRoutes) ReserveWorkLocation(ctx context.Context, id uuid.UUID, key string) (domain.ObjectLocation, error) {
	var location domain.ObjectLocation
	if s == nil || s.db == nil {
		return location, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var intent storageWorkRow
		read := tx.Raw(`SELECT * FROM media.storage_work_intent WHERE id=? FOR SHARE`, id).Scan(&intent)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 || intent.State != "open" {
			return application.ErrStorageRouteConflict
		}
		ref := domain.ObjectRef{ProfileID: intent.ProfileID, Bucket: intent.Bucket, Key: key}
		if ref.Validate() != nil {
			return domain.ErrInvalidObjectRef
		}
		var row objectLocationRow
		read = tx.Raw(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,owner_intent_id) VALUES(?,?,?,?,?) ON CONFLICT(profile_id,bucket,object_key) DO UPDATE SET updated_at=media.object_location.updated_at RETURNING *`, uuid.New(), ref.ProfileID, ref.Bucket, ref.Key, id).Scan(&row)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 || row.OwnerIntentID == nil || *row.OwnerIntentID != id {
			return application.ErrStorageRouteConflict
		}
		location = row.domain()
		return nil
	})
	return location, err
}

// BeginWorkWrite records the existing bounded execution deadline before SDK I/O.
// A paused worker cannot restart its clock after cancellation or reconciliation.
func (s *StorageRoutes) BeginWorkWrite(ctx context.Context, id uuid.UUID, location domain.ObjectLocation) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	deadline, bounded := ctx.Deadline()
	if !bounded || ctx.Err() != nil || time.Until(deadline) <= 0 || time.Until(deadline) > 5*time.Minute || location.ID == uuid.Nil || location.Ref.Validate() != nil {
		return application.ErrStorageRouteConflict
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var intent storageWorkRow
		read := tx.Raw(`SELECT * FROM media.storage_work_intent WHERE id=? FOR SHARE`, id).Scan(&intent)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 || intent.State != "open" {
			return application.ErrStorageRouteConflict
		}
		update := tx.Exec(`UPDATE media.object_location SET write_until=GREATEST(write_until,?::timestamptz) WHERE id=? AND owner_intent_id=? AND profile_id=? AND bucket=? AND object_key=? AND state IN ('reserved','present','unknown')`, deadline.UTC(), location.ID, id, location.Ref.ProfileID, location.Ref.Bucket, location.Ref.Key)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return application.ErrStorageRouteConflict
		}
		return nil
	})
}

// ConfirmWorkLocation CAS-records verified metadata while the task remains open.
func (s *StorageRoutes) ConfirmWorkLocation(ctx context.Context, id uuid.UUID, location domain.ObjectLocation, info application.ObjectInfo) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	if location.ID == uuid.Nil || location.Ref.Validate() != nil || info.Size < 0 || info.Size > 5<<30 || info.ContentType == "" || !storageDigest(info.SHA256) {
		return application.ErrObjectMismatch
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var intent storageWorkRow
		read := tx.Raw(`SELECT * FROM media.storage_work_intent WHERE id=? FOR SHARE`, id).Scan(&intent)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 || intent.State != "open" {
			return application.ErrStorageRouteConflict
		}
		update := tx.Exec(`UPDATE media.object_location SET state='present',byte_size=?,sha256=?,mime_type=? WHERE id=? AND owner_intent_id=? AND profile_id=? AND bucket=? AND object_key=? AND state IN ('reserved','present','unknown') AND (byte_size IS NULL OR byte_size=?) AND (sha256 IS NULL OR sha256=?) AND (mime_type IS NULL OR mime_type=?)`, info.Size, info.SHA256, info.ContentType, location.ID, id, location.Ref.ProfileID, location.Ref.Bucket, location.Ref.Key, info.Size, info.SHA256, info.ContentType)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return application.ErrStorageRouteConflict
		}
		return nil
	})
}

// AcceptStorageWork runs in the owning result transaction, accepting the exact
// complete output set before any compensation can acquire these rows.
func (s *StorageRoutes) AcceptStorageWork(ctx context.Context, id uuid.UUID, ids []uuid.UUID) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	if len(ids) < 1 || len(ids) > 15000 {
		return application.ErrStorageRouteConflict
	}
	seen := make(map[uuid.UUID]bool, len(ids))
	for _, object := range ids {
		if object == uuid.Nil || seen[object] {
			return application.ErrStorageRouteConflict
		}
		seen[object] = true
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var intent storageWorkRow
		read := tx.Raw(`SELECT * FROM media.storage_work_intent WHERE id=? FOR UPDATE`, id).Scan(&intent)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 || intent.State == "cancelled" {
			return application.ErrStorageRouteConflict
		}
		var rows []objectLocationRow
		if err := tx.Raw(`SELECT * FROM media.object_location WHERE owner_intent_id=? ORDER BY id LIMIT 15001 FOR UPDATE`, id).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(ids) {
			return application.ErrStorageRouteConflict
		}
		for _, row := range rows {
			if !seen[row.ID] || row.State != "present" && row.State != "accepted" {
				return application.ErrStorageRouteConflict
			}
		}
		if err := tx.Exec(`UPDATE media.object_location SET state='accepted' WHERE owner_intent_id=?`, id).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE media.storage_work_intent SET state='accepted',closed_at=COALESCE(closed_at,now()) WHERE id=?`, id).Error
	})
}

// CancelStorageWork closes future writes without declaring unknown bytes removed.
func (s *StorageRoutes) CancelStorageWork(ctx context.Context, id uuid.UUID) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	update := s.db.WithContext(ctx).Exec(`UPDATE media.storage_work_intent SET state='cancelled',closed_at=COALESCE(closed_at,now()) WHERE id=? AND state IN ('open','cancelled')`, id)
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return application.ErrStorageRouteConflict
	}
	return nil
}

// BeginWorkRemoval requires cancellation and a ten-minute drain for the bounded
// five-minute SDK write. Accepted assets and uncertain live writes stay protected.
func (s *StorageRoutes) BeginWorkRemoval(ctx context.Context, id uuid.UUID, location domain.ObjectLocation) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	remove := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var intent storageWorkRow
		read := tx.Raw(`SELECT * FROM media.storage_work_intent WHERE id=? AND state='cancelled' AND closed_at<now()-interval '10 minutes' FOR UPDATE`, id).Scan(&intent)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 {
			return nil
		}
		for _, column := range legacyObjectColumns {
			var live bool
			if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM `+column.table+` t WHERE `+column.location+`=? AND NOT(`+legacyRetired(column)+`))`, location.ID).Scan(&live).Error; err != nil {
				return err
			}
			if live {
				return nil
			}
		}
		update := tx.Exec(`UPDATE media.object_location l SET state='delete_pending' WHERE l.id=? AND l.owner_intent_id=? AND l.profile_id=? AND l.bucket=? AND l.object_key=? AND l.state IN ('reserved','present','unknown','delete_pending') AND (l.write_until IS NULL OR l.write_until<clock_timestamp()) AND NOT EXISTS(SELECT 1 FROM media.media_asset a WHERE a.object_location_id=l.id) AND NOT EXISTS(SELECT 1 FROM media.rendition r WHERE r.object_location_id=l.id)`, location.ID, id, location.Ref.ProfileID, location.Ref.Bucket, location.Ref.Key)
		remove = update.RowsAffected == 1
		return update.Error
	})
	return remove, err
}

// ConfirmWorkRemoval clears debt only for the same fenced cancelled work object.
func (s *StorageRoutes) ConfirmWorkRemoval(ctx context.Context, id uuid.UUID, location domain.ObjectLocation) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	update := s.db.WithContext(ctx).Exec(`UPDATE media.object_location SET state='deleted' WHERE id=? AND owner_intent_id=? AND profile_id=? AND bucket=? AND object_key=? AND state IN ('delete_pending','deleted')`, location.ID, id, location.Ref.ProfileID, location.Ref.Bucket, location.Ref.Key)
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return application.ErrStorageRouteConflict
	}
	return nil
}
