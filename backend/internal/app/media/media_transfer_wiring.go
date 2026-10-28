package media

import (
	"time"

	"gorm.io/gorm"

	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// ProvideMediaTransferStore assembles scoped media transfer storage.
func ProvideMediaTransferStore(database *gorm.DB) *pgmedia.TransferStore {
	return pgmedia.NewScopedTransferStore(database, NewMediaLibraryProjectAccess, provideMediaLibraryWorkGuards, time.Now, NewMediaScopeAccess)
}

// ProvideMediaTransferWorker assembles the transfer worker and its store.
func ProvideMediaTransferWorker(database *gorm.DB, storage *objectstorage.Client) (*mediaapp.TransferWorker, *pgmedia.TransferStore) {
	store := ProvideMediaTransferWorkerStore(database)
	return mediaapp.NewTransferWorker(store, mediaobjects.NewProjectCopyObjects(storage), ""), store
}

// ProvideMediaTransferWorkerStore assembles worker-authorized transfer storage.
func ProvideMediaTransferWorkerStore(database *gorm.DB) *pgmedia.TransferStore {
	return pgmedia.NewScopedTransferStore(database, NewMediaLibraryWorkerProjectAccess, provideMediaLibraryWorkGuards, time.Now, NewMediaWorkerScopeAccess)
}

// ProvideRoutedMediaTransferStore freezes each transfer item's physical target.
func ProvideRoutedMediaTransferStore(database *gorm.DB) *pgmedia.TransferStore {
	return ProvideMediaTransferStore(database).WithRoutedStorage()
}

// ProvideMediaTransferWorkerWithObjects uses only owner-fenced fixed locations.
func ProvideMediaTransferWorkerWithObjects(database *gorm.DB, registry *objectstorage.Registry, legacy *objectstorage.Client) (*mediaapp.TransferWorker, *pgmedia.TransferStore) {
	if registry == nil {
		return ProvideMediaTransferWorker(database, legacy)
	}
	store := ProvideMediaTransferWorkerStore(database).WithRoutedStorage()
	return mediaapp.NewRoutedTransferWorker(store, mediaobjects.NewRoutedCopyObjects(registry, pgmedia.NewStorageRoutes(database)), ""), store
}
