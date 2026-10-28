package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
)

// StorageFactory binds media storage ownership to the business transaction.
type StorageFactory func(*gorm.DB) application.ToolStorageLedger

// WithStorageAdmission enables frozen physical targets without changing legacy stores.
func (s *Store) WithStorageAdmission(f StorageFactory) *Store { c := *s; c.storage = f; return &c }

// WithStorageAdmission freezes depth targets in the admission transaction.
func (s *DepthStore) WithStorageAdmission(f StorageFactory) *DepthStore {
	c := *s
	c.storage = f
	return &c
}

func pinTool(ctx context.Context, tx *gorm.DB, f StorageFactory, kind string, project, job uuid.UUID, attempt int) error {
	if f == nil {
		return nil
	}
	_, e := f(tx).PinStorageWork(ctx, application.StorageOwner(kind, project, job, attempt))
	return e
}

// acceptToolMedia verifies the exact complete artifact set and attaches immutable
// locations before the owning media repository inserts its NOT NULL references.
func acceptToolMedia(ctx context.Context, tx *gorm.DB, f StorageFactory, owner media.StorageWorkOwner, a *domain.MediaAsset, rends []domain.Rendition) error {
	if f == nil {
		return nil
	}
	ledger := f(tx)
	intent, e := ledger.ReadStorageWork(ctx, owner)
	if e != nil {
		return e
	}
	if intent.Owner != owner || intent.State != "open" {
		return media.ErrStorageIdentityConflict
	}
	ids := make([]uuid.UUID, 0, len(rends)+1)
	bind := func(key string, size int64, mime, sha *string) (*domain.ObjectLocation, error) {
		l, e := ledger.ReserveWorkLocation(ctx, intent.ID, key)
		if e != nil {
			return nil, e
		}
		if l.ID == uuid.Nil || l.Ref != (domain.ObjectRef{ProfileID: intent.ProfileID, Bucket: intent.Bucket, Key: key}) || l.State != "present" || l.ByteSize == nil || *l.ByteSize != size || l.SHA256 == nil || l.MIMEType == nil || (mime != nil && *l.MIMEType != *mime) || (sha != nil && *l.SHA256 != *sha) {
			return nil, media.ErrStorageIdentityConflict
		}
		ids = append(ids, l.ID)
		return &l, nil
	}
	a.ObjectLocation, e = bind(a.ObjectKey, a.ByteSize, &a.MimeType, a.SHA256)
	if e != nil {
		return e
	}
	for i := range rends {
		if rends[i].ByteSize == nil {
			return media.ErrStorageIdentityConflict
		}
		rends[i].ObjectLocation, e = bind(rends[i].ObjectKey, *rends[i].ByteSize, nil, nil)
		if e != nil {
			return e
		}
	}
	return ledger.AcceptStorageWork(ctx, intent.ID, ids)
}

// WithStorageAdmission freezes the transcript attempt output before dispatch.
func (s *TranscriptionStore) WithStorageAdmission(f StorageFactory) *TranscriptionStore {
	c := *s
	c.storage = f
	return &c
}
