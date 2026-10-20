package catalog

import (
	"fmt"
	"io"
	"net/http"
	"os"

	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/worker"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/credentialseal"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/providerprobe"
	managedprovider "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/managed"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/staging"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

func RegisterProviderActivities(w worker.Worker, database *gorm.DB, cfg config.Config) error {
	file, err := os.Open(cfg.CredentialPrivateKeyFile)
	if err != nil {
		return fmt.Errorf("provider worker requires readable private sealing key")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 16385))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(data) > 16384 {
		return fmt.Errorf("provider worker private key unreadable or oversized")
	}
	defer clear(data)
	unsealer, err := credentialseal.NewUnsealer(cfg.CredentialKeyID, data)
	if err != nil {
		return fmt.Errorf("configure provider worker sealing key: %w", err)
	}
	activities := providerprobe.NewScopedActivities(database, unsealer, providerhttp.NewClient(&http.Client{}), operationwire.NewCatalogStore(database))
	w.RegisterActivityWithOptions(activities.TestCredential, activity.RegisterOptions{Name: "provider.test_credential"})

	storage, err := objectstorage.Open(cfg.ObjectStorageEndpoint, cfg.ObjectStorageBucket, cfg.ObjectStorageAccessKey, cfg.ObjectStorageSecretKey, cfg.ObjectStorageRegion)
	if err != nil {
		return fmt.Errorf("configure provider result storage: %w", err)
	}
	store := operationwire.ProvideOperationStore(database)
	results := operationapp.NewManagedResults(staging.NewObjects(storage), store)
	execution := managedprovider.NewActivities(store, unsealer, providerhttp.NewClient(&http.Client{}), results, storage, managedprovider.NewVideos())
	w.RegisterActivityWithOptions(execution.Step, activity.RegisterOptions{Name: "provider.managed"})
	return nil
}
