package media

import (
	"time"

	"gorm.io/gorm"

	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

func ProvideMediaTransferStore(database *gorm.DB) *pgmedia.TransferStore {
	return pgmedia.NewScopedTransferStore(database, NewMediaLibraryProjectAccess, provideMediaLibraryWorkGuards, time.Now, NewMediaScopeAccess)
}

func ProvideMediaTransferWorker(database *gorm.DB, storage *objectstorage.Client) (*mediaapp.TransferWorker, *pgmedia.TransferStore) {
	store := ProvideMediaTransferWorkerStore(database)
	return mediaapp.NewTransferWorker(store, mediaobjects.NewProjectCopyObjects(storage), ""), store
}

func ProvideMediaTransferWorkerStore(database *gorm.DB) *pgmedia.TransferStore {
	return pgmedia.NewScopedTransferStore(database, NewMediaLibraryWorkerProjectAccess, provideMediaLibraryWorkGuards, time.Now, NewMediaWorkerScopeAccess)
}
