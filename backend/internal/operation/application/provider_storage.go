package application

import (
	"context"

	"github.com/google/uuid"
)

// ProviderStagingFactory binds each execution/recovery to an already admitted
// storage target. It must not choose the current default while executing a job.
type ProviderStagingFactory interface {
	ForProviderDispatch(context.Context, ProviderDispatchIdentity) (ProviderStagingObjects, error)
}

// OperationStorageAdmission pins the output target in the quote's owning SQL
// transaction, before any provider dispatch or asynchronous result is possible.
type OperationStorageAdmission interface {
	PinOperationStorage(context.Context, uuid.UUID, uuid.UUID) error
}
