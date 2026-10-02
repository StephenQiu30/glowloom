package app

import (
	"time"

	"gorm.io/gorm"

	mediaobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

func provideMediaTransferStore(database *gorm.DB) *pgmedia.TransferStore {
	return pgmedia.NewTransferStore(database, provideMediaLibraryProjectAccess, provideMediaLibraryWorkGuards, time.Now)
}

func provideMediaTransferWorker(database *gorm.DB, storage *objectstorage.Client) (*mediaapp.TransferWorker, *pgmedia.TransferStore) {
	store := provideMediaTransferStore(database)
	return mediaapp.NewTransferWorker(store, mediaobjects.NewProjectCopyObjects(storage), ""), store
}
