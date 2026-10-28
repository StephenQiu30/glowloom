package application

import (
	"context"
	"strconv"

	"github.com/google/uuid"

	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// StorageOwner isolates output identity for each durable tool attempt.
func StorageOwner(kind string, project, job uuid.UUID, attempt int) media.StorageWorkOwner {
	return media.StorageWorkOwner{Kind: kind, OwnerID: project, WorkID: uuid.NewSHA1(job, []byte("storage-attempt/"+strconv.Itoa(attempt)))}
}

// WorkObjectsFactory creates a private, concurrency-safe object view per attempt.
// Inputs carry locations revalidated by the owning media port; no default route is used.
type WorkObjectsFactory interface {
	Open(context.Context, media.StorageWorkOwner, []domain.MediaAsset) (DepthObjects, error)
}

// ToolStorageLedger is supplied by the owning media application at composition.
type ToolStorageLedger interface {
	media.StorageWorkAdmission
	media.WorkObjectLocations
	ReadLocation(context.Context, uuid.UUID) (domain.ObjectLocation, error)
	AcceptStorageWork(context.Context, uuid.UUID, []uuid.UUID) error
	CancelStorageWork(context.Context, uuid.UUID) error
	FindWorkLocation(context.Context, uuid.UUID, string) (domain.ObjectLocation, bool, error)
}
