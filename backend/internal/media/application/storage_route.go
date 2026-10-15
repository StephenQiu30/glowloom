package application

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

var (
	// ErrStorageRouteConflict rejects a stale or reused activation revision.
	ErrStorageRouteConflict = errors.New("storage route revision conflict")
	// ErrStorageIdentityConflict prevents rebinding or aliasing a physical resource.
	ErrStorageIdentityConflict = errors.New("storage profile identity conflict")
	// ErrStorageNotReady prevents use before migration and capabilities are accepted.
	ErrStorageNotReady = errors.New("storage migration or capability not accepted")
)

// StorageProfile is the public identity persisted for a deployment resource.
// Credential references and temporary access URLs never enter the database.
type StorageProfile struct {
	ID, Provider, Endpoint, SigningEndpoint, Region, Bucket, AddressingStyle string
}

// StorageRoute is the database-selected destination for future write intents.
type StorageRoute struct{ ProfileID, Bucket, Revision string }

// StorageRouteRepository exposes a durable read and explicit compare-and-swap.
type StorageRouteRepository interface {
	ReadActive(context.Context) (StorageRoute, error)
	Activate(context.Context, string, string, string) (StorageRoute, error)
}

// ObjectLocations owns stable identity independently of the route used for new work.
type ObjectLocations interface {
	ReserveLocation(context.Context, domain.ObjectRef) (domain.ObjectLocation, error)
	ReadLocation(context.Context, uuid.UUID) (domain.ObjectLocation, error)
}
