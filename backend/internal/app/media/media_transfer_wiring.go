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

// ProvideMediaTransferWorkerStore assembles worker-authorized transfer storage.
func ProvideMediaTransferWorkerStore(database *gorm.DB) *pgmedia.TransferStore {
	return pgmedia.NewScopedTransferStore(database, NewMediaLibraryWorkerProjectAccess, provideMediaLibraryWorkGuards, time.Now, NewMediaWorkerScopeAccess)
}

// ProvideRoutedMediaTransferStore freezes each transfer item's physical target.
func ProvideRoutedMediaTransferStore(database *gorm.DB) *pgmedia.TransferStore {
	return ProvideMediaTransferStore(database).WithRoutedStorage()
}

// ProvideMediaTransferWorker uses only owner-fenced fixed locations.
func ProvideMediaTransferWorker(database *gorm.DB, registry *objectstorage.Registry) (*mediaapp.TransferWorker, *pgmedia.TransferStore) {
	store := ProvideMediaTransferWorkerStore(database).WithRoutedStorage()
	return mediaapp.NewRoutedTransferWorker(store, mediaobjects.NewRoutedCopyObjects(registry, pgmedia.NewStorageRoutes(database)), ""), store
}
