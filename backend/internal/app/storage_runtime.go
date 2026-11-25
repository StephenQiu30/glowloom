package app

import (
	"context"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/storageoperator"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// startStorageRuntime applies the deployment's normalized storage configuration
// before serving requests. The environment is the only source of route changes.
func startStorageRuntime(ctx context.Context, cfg config.Config, required bool, database *gorm.DB) (func(), error) {
	if cfg.ObjectStorage == nil {
		if required {
			return nil, mediaapp.ErrStorageNotReady
		}
		return func() {}, nil
	}
	if database == nil {
		return nil, mediaapp.ErrUnavailable
	}
	environment, err := storageoperator.Configuration(*cfg.ObjectStorage)
	if err != nil {
		return nil, err
	}
	registry, err := objectstorage.OpenProfiles(*cfg.ObjectStorage)
	if err != nil {
		return nil, err
	}
	defer registry.Close()
	if err := registry.PingProfile(ctx, environment.DefaultWriteProfile); err != nil {
		return nil, err
	}
	if _, err := pgmedia.NewStorageRoutes(database).ApplyEnvironment(ctx, environment); err != nil {
		return nil, err
	}
	return func() {}, nil
}
