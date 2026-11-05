//go:build wireinject

package app

import (
	"context"
	"net/http"

	"github.com/google/wire"
	"go.uber.org/zap"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func initializeAPI(ctx context.Context, cfg config.Config, logger *zap.Logger) (*http.Server, func(), error) {
	wire.Build(provideTrace, provideDB, provideRedis, provideObjectStorage, provideReadyCheck, provideAPIServer)
	return nil, nil, nil
}

func initializeWorker(ctx context.Context, cfg config.Config, logger *zap.Logger, queue string) (*taskruntime.Worker, func(), error) {
	wire.Build(provideWorkerTrace, provideDB, provideBackendWorker)
	return nil, nil, nil
}

func initializeRelay(ctx context.Context, cfg config.Config, logger *zap.Logger) (*relayRuntime, func(), error) {
	wire.Build(provideRelayTrace, provideDB, provideKafka, provideRedis, provideRelayRuntime)
	return nil, nil, nil
}
