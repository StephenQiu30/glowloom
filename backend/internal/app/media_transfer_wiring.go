package app

import (
	"time"

	"gorm.io/gorm"

	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

func provideMediaTransferStore(database *gorm.DB) *pgmedia.TransferStore {
	return pgmedia.NewScopedTransferStore(database, NewMediaLibraryProjectAccess, provideMediaLibraryWorkGuards, time.Now, provideMediaScopeAccess)
}

func provideMediaTransferWorker(database *gorm.DB, storage *objectstorage.Client) (*mediaapp.TransferWorker, *pgmedia.TransferStore) {
	store := provideMediaTransferWorkerStore(database)
	return mediaapp.NewTransferWorker(store, mediaobjects.NewProjectCopyObjects(storage), ""), store
}

func provideMediaTransferWorkerStore(database *gorm.DB) *pgmedia.TransferStore {
	return pgmedia.NewScopedTransferStore(database, provideMediaLibraryWorkerProjectAccess, provideMediaLibraryWorkGuards, time.Now, provideMediaWorkerScopeAccess)
}
