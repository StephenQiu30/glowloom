package application

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

var (
	// ErrStorageRouteConflict rejects an inconsistent environment revision.
	ErrStorageRouteConflict = errors.New("storage route revision conflict")
	// ErrStorageIdentityConflict prevents rebinding or aliasing a physical resource.
	ErrStorageIdentityConflict = errors.New("storage profile identity conflict")
	// ErrStorageNotReady prevents use before environment routing is configured.
	ErrStorageNotReady = errors.New("storage migration or capability not accepted")
)

// StorageProfile is the public identity persisted for a deployment resource.
// Credential references and temporary access URLs never enter the database.
type StorageProfile struct {
	ID, Provider, Endpoint, SigningEndpoint, Region, Bucket, AddressingStyle string
}

// StorageRoute is the database projection of the environment write destination.
type StorageRoute struct{ ProfileID, Bucket, Revision string }

// StorageRouteRepository exposes the environment route projection.
type StorageRouteRepository interface {
	ReadConfigured(context.Context) (StorageRoute, error)
}

// ObjectLocations owns stable identity independently of the route used for new work.
type ObjectLocations interface {
	ReserveLocation(context.Context, domain.ObjectRef) (domain.ObjectLocation, error)
	ReadLocation(context.Context, uuid.UUID) (domain.ObjectLocation, error)
}
