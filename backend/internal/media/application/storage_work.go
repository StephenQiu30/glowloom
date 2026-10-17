package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// StorageWorkOwner is the owning module's immutable job identity. OwnerID closes
// request IDs that are unique only per actor or project.
type StorageWorkOwner struct {
	Kind            string
	OwnerID, WorkID uuid.UUID
}

// Validate rejects arbitrary namespaces and incomplete task ownership.
func (o StorageWorkOwner) Validate() error {
	if o.OwnerID == uuid.Nil || o.WorkID == uuid.Nil {
		return ErrStorageRouteConflict
	}
	switch o.Kind {
	case "operation", "script", "export", "depth", "transcription", "package", "transfer", "project-copy":
		return nil
	default:
		return ErrStorageRouteConflict
	}
}

// StorageWorkIntent freezes the accepted write profile for every output of a job.
type StorageWorkIntent struct {
	ID                                       uuid.UUID
	Owner                                    StorageWorkOwner
	ProfileID, Bucket, ConfigRevision, State string
}

// StorageWorkAdmission is called within the business owner's admission transaction.
type StorageWorkAdmission interface {
	PinStorageWork(context.Context, StorageWorkOwner) (StorageWorkIntent, error)
	ReadStorageWork(context.Context, StorageWorkOwner) (StorageWorkIntent, error)
}

// WorkObjectLocations records intent before network I/O and fences acceptance
// against compensating removal. Callers still enforce their business worker lease.
type WorkObjectLocations interface {
	ReserveWorkLocation(context.Context, uuid.UUID, string) (domain.ObjectLocation, error)
	BeginWorkWrite(context.Context, uuid.UUID, domain.ObjectLocation) error
	ConfirmWorkLocation(context.Context, uuid.UUID, domain.ObjectLocation, ObjectInfo) error
	BeginWorkRemoval(context.Context, uuid.UUID, domain.ObjectLocation) (bool, error)
	ConfirmWorkRemoval(context.Context, uuid.UUID, domain.ObjectLocation) error
}
