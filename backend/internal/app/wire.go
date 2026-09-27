//go:build wireinject

package app

import (
	"context"
	"net/http"

	"github.com/google/wire"
	"go.temporal.io/sdk/worker"
	"go.uber.org/zap"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	maintenanceflow "github.com/StephenQiu30/lanverse/backend/internal/infra/maintenance/adapter/temporal"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
)

func initializeAPI(ctx context.Context, cfg config.Config, logger *zap.Logger) (*http.Server, func(), error) {
	wire.Build(provideTrace, provideDB, provideRedis, provideTemporal, provideObjectStorage, provideReadyCheck, provideAPIServer)
	return nil, nil, nil
}

func initializeWorker(ctx context.Context, cfg config.Config, logger *zap.Logger, queue string) (worker.Worker, func(), error) {
	wire.Build(provideWorkerTrace, provideDB, provideTemporal, provideBackendWorker)
	return nil, nil, nil
}

func initializeRelay(ctx context.Context, cfg config.Config, logger *zap.Logger) (*relayRuntime, func(), error) {
	wire.Build(provideRelayTrace, provideDB, provideKafka, provideRedis, provideRelayRuntime)
	return nil, nil, nil
}

func initializeMaintenanceSetup(ctx context.Context, cfg config.Config, logger *zap.Logger, prefix string) (*maintenanceflow.ScheduleInstaller, func(), error) {
	wire.Build(provideSetupTrace, provideTemporal, provideCleanupScheduleInstaller)
	return nil, nil, nil
}

func initializeAdminBootstrap(ctx context.Context, cfg config.Config, logger *zap.Logger) (*identityapp.BootstrapAdminCommand, func(), error) {
	wire.Build(provideBootstrapTrace, provideDB, provideIdentityStore, provideBootstrapAdminCommand)
	return nil, nil, nil
}
