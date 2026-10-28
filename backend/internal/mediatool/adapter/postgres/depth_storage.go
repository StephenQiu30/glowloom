package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
)

func (s *DepthStore) reserveDepthObject(ctx context.Context, tx *gorm.DB, row depthRow, key string) (*uuid.UUID, error) {
	if s.storage == nil {
		return nil, nil
	}
	ledger := s.storage(tx)
	owner := application.StorageOwner("depth", row.ProjectID, row.ID, row.Attempt)
	intent, err := ledger.ReadStorageWork(ctx, owner)
	if err != nil {
		return nil, err
	}
	if intent.Owner != owner || intent.State != "open" {
		return nil, mediaapp.ErrStorageIdentityConflict
	}
	location, err := ledger.ReserveWorkLocation(ctx, intent.ID, key)
	if err != nil {
		return nil, err
	}
	if location.ID == uuid.Nil || location.Ref != (mediadomain.ObjectRef{ProfileID: intent.ProfileID, Bucket: intent.Bucket, Key: key}) {
		return nil, mediaapp.ErrStorageIdentityConflict
	}
	return &location.ID, nil
}

// verifyDepthStorage compares all physical facts with the frozen manifest before
// generic media acceptance; rendition hashes must not be inferred from size alone.
func (s *DepthStore) verifyDepthStorage(ctx context.Context, tx *gorm.DB, row depthRow) error {
	if s.storage == nil {
		return nil
	}
	ledger := s.storage(tx)
	owner := application.StorageOwner("depth", row.ProjectID, row.ID, row.Attempt)
	intent, err := ledger.ReadStorageWork(ctx, owner)
	if err != nil {
		return err
	}
	if intent.Owner != owner || intent.State != "open" {
		return mediaapp.ErrStorageIdentityConflict
	}
	objects, err := readDepthObjects(tx, row)
	if err != nil {
		return err
	}
	for _, object := range objects {
		location, err := ledger.ReserveWorkLocation(ctx, intent.ID, object.ObjectKey)
		if err != nil {
			return err
		}
		if object.ObjectLocationID == nil || location.ID != *object.ObjectLocationID || location.Ref != (mediadomain.ObjectRef{ProfileID: intent.ProfileID, Bucket: intent.Bucket, Key: object.ObjectKey}) || location.State != "present" || location.ByteSize == nil || *location.ByteSize != object.ByteSize || location.SHA256 == nil || *location.SHA256 != object.SHA256 || location.MIMEType == nil || *location.MIMEType != object.MIMEType {
			return mediaapp.ErrStorageIdentityConflict
		}
	}
	return nil
}
