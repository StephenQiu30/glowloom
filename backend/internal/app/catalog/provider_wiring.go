// Package catalog wires provider worker activities.
package catalog

import (
	"context"
	"fmt"
	"net/http"

	trialwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/aitrial"

	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/worker"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/credentialseal"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/providerprobe"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	managedprovider "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/managed"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/staging"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/adapter/doubao"
	subjectflow "github.com/StephenQiu30/glowloom-server/backend/internal/subject/adapter/workflow"
)

// RegisterProviderActivities registers provider activities with the operation worker.
func RegisterProviderActivities(ctx context.Context, w worker.Worker, database *gorm.DB, cfg config.Config) error {
	data, err := cfg.CredentialPrivateBytes()
	if err != nil {
		return fmt.Errorf("provider worker requires configured private sealing PEM: %w", err)
	}
	defer clear(data)
	unsealer, err := credentialseal.NewUnsealer(cfg.CredentialKeyID, data)
	if err != nil {
		return fmt.Errorf("configure provider worker sealing key: %w", err)
	}
	activities := providerprobe.NewScopedActivities(database, unsealer, providerhttp.NewClient(&http.Client{}), operationwire.NewCatalogStore(database))
	w.RegisterActivityWithOptions(activities.TestCredential, activity.RegisterOptions{Name: "provider.test_credential"})

	storage, err := objectstorage.OpenLegacy(cfg)
	if err != nil {
		return fmt.Errorf("configure provider result storage: %w", err)
	}
	subjectObjects, err := subjectwire.OpenObjects(cfg, storage)
	if err != nil {
		return fmt.Errorf("configure subject input transport: %w", err)
	}
	context.AfterFunc(ctx, subjectObjects.Close)
	store := operationwire.ProvideOperationStore(database).WithSubjectMedia(func(tx *gorm.DB) operationapp.SubjectMediaAccess {
		return subjectwire.NewGenerationAccess(tx, subjectObjects)
	})
	if subjectObjects.Registry() != nil {
		store.WithProjectMedia(func(*gorm.DB) operationapp.ProjectMediaSigner {
			return operationwire.NewProjectMediaSigner(subjectObjects.Registry(), subjectObjects.Legacy)
		})
	}
	voiceExecution := subjectwire.NewVoiceExecutionWithObjects(database, subjectObjects, mediaflow.FFProber{}, mediaflow.FFUploadRenderer{}, unsealer, doubao.NewSpeech(providerhttp.NewClient(&http.Client{})))
	subjectflow.NewActivities(voiceExecution).RegisterActivities(w)
	if subjectObjects.Registry() == nil && storage == nil {
		w.RegisterActivityWithOptions(func(context.Context, operationapp.ManagedStep) (operationapp.ManagedProgress, error) {
			return operationapp.ManagedProgress{}, mediaapp.ErrStorageNotReady
		}, activity.RegisterOptions{Name: "provider.managed"})
		return nil
	}
	results := operationapp.NewManagedResults(staging.NewObjects(storage), store)
	if subjectObjects.Registry() != nil {
		results = operationapp.NewRoutedManagedResults(staging.NewRoutedFactory(subjectObjects.Registry(), pgmedia.NewStorageRoutes(database)), store)
	}
	execution := managedprovider.NewActivities(store, unsealer, providerhttp.NewClient(&http.Client{}), results, storage, managedprovider.NewVideos())
	w.RegisterActivityWithOptions(execution.Step, activity.RegisterOptions{Name: "provider.managed"})
	if cfg.Env == "local" && storage != nil {
		trialwire.RegisterCloud(w, database, unsealer, providerhttp.NewClient(&http.Client{}), storage, operationapp.TrialStorageID(cfg.ObjectStorageEndpoint, cfg.ObjectStorageBucket))
	}
	return nil
}
