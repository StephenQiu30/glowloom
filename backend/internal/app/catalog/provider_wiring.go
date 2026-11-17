// Package catalog wires the rebuilt AI provider worker and read-only adapter metadata.
package catalog

import (
	"context"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/catalog/adapter/credentialschema"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/providerhttp"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// RegisterProvider installs only the new request/hold/attempt lifecycle. Retired paid dispatchers are not registered.
func RegisterProvider(ctx context.Context, handlers map[string]taskruntime.Handler, database *gorm.DB, cfg config.Config) error {
	runtime, err := aigateway.New(database, cfg)
	if err != nil {
		return err
	}
	objects, err := subjectwire.OpenObjects(cfg)
	if err != nil {
		return err
	}
	context.AfterFunc(ctx, objects.Close)
	runtime.Objects = objects.Registry()
	runtime.RegisterWorker(handlers)
	return nil
}

// InstalledProtocol reports whether the existing pure wire driver is installed.
func InstalledProtocol(id string) bool { _, ok := providerhttp.GetDriver(id); return ok }

// NewAdapterMetadata retains read-only transport descriptions for non-AI consumers.
func NewAdapterMetadata(access catalogapp.AdapterMetadataAccess) *catalogapp.AdapterMetadataQuery {
	return catalogapp.NewAdapterMetadataQuery(access, credentialschema.NewRegistry(), InstalledProtocol)
}
