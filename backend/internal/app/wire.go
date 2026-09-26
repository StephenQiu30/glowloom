//go:build wireinject

package app

import (
	"context"
	"net/http"

	"github.com/google/wire"
	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
)

func initializeAPI(ctx context.Context, cfg config.Config, logger *zap.Logger) (*http.Server, func(), error) {
	wire.Build(provideDB, provideRedis, provideTemporal, provideReadyCheck, provideAPIServer)
	return nil, nil, nil
}
