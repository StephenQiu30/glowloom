package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/otelconn"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/redisconn"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/temporalconn"
)

func provideTrace(ctx context.Context, cfg config.Config, logger *zap.Logger) (trace.TracerProvider, func(), error) {
	provider, shutdown, err := otelconn.Open(ctx, cfg.OTelEndpoint, "lanverse-backend-api")
	if err != nil {
		return nil, nil, fmt.Errorf("configure api tracing: %w", err)
	}
	cleanup := func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := shutdown(shutdownCtx); err != nil {
			logger.Error("flush api tracing", zap.Error(err))
		}
	}
	return provider, cleanup, nil
}

func provideDB(ctx context.Context, cfg config.Config, logger *zap.Logger, tracerProvider trace.TracerProvider) (*db.Connection, func(), error) {
	conn, err := db.Open(ctx, cfg.DBDSN, tracerProvider)
	if err != nil {
		return nil, nil, fmt.Errorf("connect api database: %w", err)
	}
	cleanup := func() {
		if err := conn.Close(); err != nil {
			logger.Error("close api database", zap.Error(err))
		}
	}
	return conn, cleanup, nil
}

func provideRedis(cfg config.Config, logger *zap.Logger) (*redisconn.Connection, func(), error) {
	redisconn.ConfigureLogging(logger)
	conn, err := redisconn.Open(cfg.RedisURL)
	if err != nil {
		return nil, nil, fmt.Errorf("configure api Redis: %w", err)
	}
	cleanup := func() {
		if err := conn.Close(); err != nil {
			logger.Error("close api Redis", zap.Error(err))
		}
	}
	return conn, cleanup, nil
}

func provideTemporal(cfg config.Config, logger *zap.Logger, tracerProvider trace.TracerProvider) (*temporalconn.Connection, func(), error) {
	conn, err := temporalconn.Open(cfg.TemporalAddr, cfg.TemporalNamespace, logger, tracerProvider)
	if err != nil {
		return nil, nil, fmt.Errorf("configure api Temporal: %w", err)
	}
	return conn, conn.Close, nil
}

func provideObjectStorage(cfg config.Config) (*objectstorage.Client, error) {
	conn, err := objectstorage.Open(cfg.ObjectStorageEndpoint, cfg.ObjectStorageBucket, cfg.ObjectStorageAccessKey, cfg.ObjectStorageSecretKey, cfg.ObjectStorageRegion)
	if err != nil {
		return nil, fmt.Errorf("configure api object storage: %w", err)
	}
	return conn, nil
}

func provideReadyCheck(dbConn *db.Connection, redisConn *redisconn.Connection, temporalConn *temporalconn.Connection, storageClient *objectstorage.Client) ReadyCheck {
	return func(ctx context.Context) error {
		if err := dbConn.Ping(ctx); err != nil {
			return err
		}
		if err := redisConn.Ping(ctx); err != nil {
			return err
		}
		if err := temporalConn.Ping(ctx); err != nil {
			return err
		}
		return storageClient.Ping(ctx)
	}
}

func provideAPIServer(cfg config.Config, logger *zap.Logger, ready ReadyCheck, tracerProvider trace.TracerProvider) *http.Server {
	return &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           NewRouter(logger, ready, tracerProvider),
		ReadHeaderTimeout: 10 * time.Second,
	}
}
