// Package catalog wires provider worker activities.
package catalog

import (
	"context"
	"fmt"
	"net/http"

	catalogtask "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/task"
	operationtask "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/task"
	trialtask "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/trialtask"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	subjecttask "github.com/StephenQiu30/glowloom-server/backend/internal/subject/adapter/task"

	trialwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/aitrial"

	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/credentialschema"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/credentialseal"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/providerprobe"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	managedprovider "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/managed"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/staging"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/adapter/doubao"
)

// RegisterProvider registers provider activities with the operation worker.
func RegisterProvider(ctx context.Context, handlers map[string]taskruntime.Handler, database *gorm.DB, cfg config.Config) error {
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
	catalogStore := operationwire.NewCatalogStore(database)
	handlers[catalogtask.Credential] = catalogtask.CredentialHandler(catalogtask.NewActivities(catalogapp.NewCredentialTestService(catalogStore), catalogStore), activities)

	subjectObjects, err := subjectwire.OpenObjects(cfg)
	if err != nil {
		return fmt.Errorf("configure subject input transport: %w", err)
	}
	context.AfterFunc(ctx, subjectObjects.Close)
	registry := subjectObjects.Registry()
	if registry == nil {
		return objectstorage.ErrUnavailable
	}
	store := operationwire.ProvideOperationStore(database).WithSubjectMedia(func(tx *gorm.DB) operationapp.SubjectMediaAccess {
		return subjectwire.NewGenerationAccess(tx, subjectObjects)
	})
	store.WithProjectMedia(func(*gorm.DB) operationapp.ProjectMediaSigner {
		return operationwire.NewProjectMediaSigner(registry)
	})
	voiceExecution := subjectwire.NewVoiceExecutionWithObjects(database, subjectObjects, mediaflow.FFProber{}, mediaflow.FFUploadRenderer{}, unsealer, doubao.NewSpeech(providerhttp.NewClient(&http.Client{})))
	handlers[subjecttask.Voice] = subjecttask.VoiceHandler(voiceExecution)
	results := operationapp.NewRoutedManagedResults(staging.NewRoutedFactory(registry, pgmedia.NewStorageRoutes(database)), store)
	execution := managedprovider.NewActivities(store, unsealer, providerhttp.NewClient(&http.Client{}), results, managedprovider.NewVideos())
	operationtask.RegisterProvider(handlers, store, execution)
	if cfg.Env == "local" {
		handlers[trialtask.Cloud] = trialwire.Cloud(database, unsealer, providerhttp.NewClient(&http.Client{}), registry)
	}
	return nil
}

// InstalledProtocol resolves the existing provider driver registry without exposing drivers to catalog.
func InstalledProtocol(id string) bool { _, ok := providerhttp.GetDriver(id); return ok }

// NewAdapterMetadata injects the existing contracts and live authorization.
func NewAdapterMetadata(access catalogapp.AdapterMetadataAccess) *catalogapp.AdapterMetadataQuery {
	return catalogapp.NewAdapterMetadataQuery(access, credentialschema.NewRegistry(), InstalledProtocol)
}
