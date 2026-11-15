package media

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

// NewIdentityExportLedger binds the legacy local Media ledger to the caller's
// transaction. It preserves existing atomicity during ownership migration; it
// must not be replaced by network I/O inside the identity transaction.
func NewIdentityExportLedger(tx *gorm.DB) pgidentity.ExportObjectLedger {
	return &identityExportLedger{routes: pgmedia.NewStorageRoutes(tx)}
}

type identityExportLedger struct{ routes *pgmedia.StorageRoutes }

func exportStorageOwner(job identityapp.DirectoryExportJob) mediaapp.StorageWorkOwner {
	return mediaapp.StorageWorkOwner{Kind: mediaapp.StorageWorkKindExport, OwnerID: job.ActorID, WorkID: job.ID}
}

func exportLocationRef(location identityapp.ExportObjectLocation) mediadomain.ObjectRef {
	return mediadomain.ObjectRef{ProfileID: location.ProfileID, Bucket: location.Bucket, Key: location.Key}
}

func exportDomainLocation(location identityapp.ExportObjectLocation) mediadomain.ObjectLocation {
	return mediadomain.ObjectLocation{ID: location.ID, Ref: exportLocationRef(location)}
}

func (s *identityExportLedger) PinExportObject(ctx context.Context, job identityapp.DirectoryExportJob) (identityapp.ExportObjectLocation, error) {
	if job.ObjectID == nil || job.ID == uuid.Nil || job.ActorID == uuid.Nil {
		return identityapp.ExportObjectLocation{}, identityapp.ErrExportUnavailable
	}
	intent, err := s.routes.PinStorageWork(ctx, exportStorageOwner(job))
	if err != nil {
		return identityapp.ExportObjectLocation{}, err
	}
	key := "private/identity/directory-exports/" + job.ObjectID.String() + ".enc"
	location, err := s.routes.ReserveWorkLocation(ctx, intent.ID, key)
	if err != nil {
		return identityapp.ExportObjectLocation{}, err
	}
	return identityapp.ExportObjectLocation{ID: location.ID, IntentID: intent.ID, ProfileID: location.Ref.ProfileID, Bucket: location.Ref.Bucket, Key: location.Ref.Key}, nil
}

func (s *identityExportLedger) exportWork(ctx context.Context, job identityapp.DirectoryExportJob) (mediaapp.StorageWorkIntent, mediadomain.ObjectLocation, error) {
	if job.ObjectLocation == nil || job.ObjectLocation.ID == uuid.Nil || job.ObjectLocation.IntentID == uuid.Nil {
		return mediaapp.StorageWorkIntent{}, mediadomain.ObjectLocation{}, identityapp.ErrExportUnavailable
	}
	intent, err := s.routes.ReadStorageWork(ctx, exportStorageOwner(job))
	if err != nil {
		return intent, mediadomain.ObjectLocation{}, err
	}
	if intent.ID != job.ObjectLocation.IntentID || intent.ProfileID != job.ObjectLocation.ProfileID || intent.Bucket != job.ObjectLocation.Bucket {
		return intent, mediadomain.ObjectLocation{}, identityapp.ErrExportUnavailable
	}
	location, err := s.routes.ReadLocation(ctx, job.ObjectLocation.ID)
	if err != nil {
		return intent, location, err
	}
	if location.Ref != exportLocationRef(*job.ObjectLocation) {
		return intent, location, identityapp.ErrExportUnavailable
	}
	return intent, location, nil
}

func (s *identityExportLedger) BeginExportObjectWrite(ctx context.Context, job identityapp.DirectoryExportJob, deadline time.Time) error {
	intent, location, err := s.exportWork(ctx, job)
	if err != nil {
		return err
	}
	bounded, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	return s.routes.BeginWorkWrite(bounded, intent.ID, location)
}

func (s *identityExportLedger) ConfirmExportObject(ctx context.Context, job identityapp.DirectoryExportJob, info identityapp.ExportObjectInfo) error {
	intent, location, err := s.exportWork(ctx, job)
	if err != nil {
		return err
	}
	return s.routes.ConfirmWorkLocation(ctx, intent.ID, location, mediaapp.ObjectInfo{Size: info.Size, ContentType: info.ContentType, SHA256: info.SHA256})
}

func (s *identityExportLedger) AcceptExportObject(ctx context.Context, job identityapp.DirectoryExportJob) error {
	intent, location, err := s.exportWork(ctx, job)
	if err != nil {
		return err
	}
	return s.routes.AcceptStorageWork(ctx, intent.ID, []uuid.UUID{location.ID})
}

func (s *identityExportLedger) CancelExportObject(ctx context.Context, job identityapp.DirectoryExportJob) error {
	if job.ObjectLocation == nil || job.ObjectLocation.IntentID == uuid.Nil {
		return nil
	}
	intent, err := s.routes.ReadStorageWork(ctx, exportStorageOwner(job))
	if err != nil {
		return err
	}
	if intent.ID != job.ObjectLocation.IntentID {
		return identityapp.ErrExportUnavailable
	}
	return s.routes.CancelStorageWork(ctx, intent.ID)
}

func (s *identityExportLedger) BeginExportObjectRemoval(ctx context.Context, job identityapp.DirectoryExportJob) (identityapp.ExportObjectLocation, bool, error) {
	if job.ObjectLocation == nil || job.ObjectLocation.IntentID == uuid.Nil {
		return identityapp.ExportObjectLocation{}, false, identityapp.ErrExportUnavailable
	}
	location := *job.ObjectLocation
	physical, err := s.routes.ReadLocation(ctx, location.ID)
	if err != nil {
		return location, false, err
	}
	if physical.Ref != exportLocationRef(location) {
		return location, false, identityapp.ErrExportUnavailable
	}
	if physical.State == mediadomain.ObjectLocationDeleted {
		return location, true, nil
	}
	intent, err := s.routes.ReadStorageWork(ctx, exportStorageOwner(job))
	if err != nil {
		return location, false, err
	}
	if intent.ID != location.IntentID {
		return location, false, identityapp.ErrExportUnavailable
	}
	var allowed bool
	switch intent.State {
	case mediaapp.StorageWorkOpen:
		if err := s.routes.CancelStorageWork(ctx, intent.ID); err != nil {
			return location, false, err
		}
		allowed, err = s.routes.BeginWorkRemoval(ctx, intent.ID, physical)
	case mediaapp.StorageWorkCancelled:
		allowed, err = s.routes.BeginWorkRemoval(ctx, intent.ID, physical)
	case mediaapp.StorageWorkAccepted:
		allowed, err = s.routes.BeginAcceptedWorkRemoval(ctx, intent.ID, physical)
	default:
		return location, false, identityapp.ErrExportUnavailable
	}
	return location, allowed, err
}

func (s *identityExportLedger) ConfirmExportObjectRemoval(ctx context.Context, _ identityapp.DirectoryExportJob, location identityapp.ExportObjectLocation) error {
	if location.ID == uuid.Nil || location.IntentID == uuid.Nil {
		return identityapp.ErrExportUnavailable
	}
	return s.routes.ConfirmWorkRemoval(ctx, location.IntentID, exportDomainLocation(location))
}
