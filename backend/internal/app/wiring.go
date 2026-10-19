package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityruntime"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/otelconn"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/redisconn"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/temporalconn"
)

func provideTrace(ctx context.Context, cfg config.Config, logger *zap.Logger) (trace.TracerProvider, func(), error) {
	return provideNamedTrace(ctx, cfg, logger, "api")
}

func provideNamedTrace(ctx context.Context, cfg config.Config, logger *zap.Logger, role string) (trace.TracerProvider, func(), error) {
	provider, shutdown, err := otelconn.Open(ctx, cfg.OTelEndpoint, "glowloom-backend-"+role)
	if err != nil {
		return nil, nil, fmt.Errorf("configure %s tracing: %w", role, err)
	}
	cleanup := func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := shutdown(shutdownCtx); err != nil {
			logger.Error("flush tracing", zap.String("role", role), zap.Error(err))
		}
	}
	return provider, cleanup, nil
}

func provideDB(ctx context.Context, cfg config.Config, logger *zap.Logger, tracerProvider trace.TracerProvider) (*db.Connection, func(), error) {
	conn, err := db.Open(ctx, cfg.DBDSN, tracerProvider)
	if err != nil {
		return nil, nil, fmt.Errorf("connect backend database: %w", err)
	}
	cleanup := func() {
		if err := conn.Close(); err != nil {
			logger.Error("close backend database", zap.Error(err))
		}
	}
	return conn, cleanup, nil
}

func provideRedis(cfg config.Config, logger *zap.Logger) (*redisconn.Connection, func(), error) {
	conn, err := redisconn.Open(cfg.RedisURL)
	if err != nil {
		return nil, nil, fmt.Errorf("configure backend Redis: %w", err)
	}
	cleanup := func() {
		if err := conn.Close(); err != nil {
			logger.Error("close backend Redis", zap.Error(err))
		}
	}
	return conn, cleanup, nil
}

func provideTemporal(cfg config.Config, logger *zap.Logger, tracerProvider trace.TracerProvider) (*temporalconn.Connection, func(), error) {
	conn, err := temporalconn.Open(cfg.TemporalAddr, cfg.TemporalNamespace, logger, tracerProvider)
	if err != nil {
		return nil, nil, fmt.Errorf("configure backend Temporal: %w", err)
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

func provideAPIServer(ctx context.Context, cfg config.Config, logger *zap.Logger, ready ReadyCheck, tracerProvider trace.TracerProvider, dbConn *db.Connection, storage *objectstorage.Client, redisConn *redisconn.Connection) (*http.Server, func(), error) {
	sessions, err := identityruntime.NewSessionDependencies(cfg, redisConn.Client)
	if err != nil {
		return nil, nil, err
	}
	router, err := NewBusinessRouter(logger, ready, tracerProvider, cfg, dbConn.DB, storage, sessions)
	if err != nil {
		return nil, nil, err
	}
	cleanup, err := identityruntime.StartMailLoop(ctx, dbConn.DB, cfg, logger)
	if err != nil {
		return nil, nil, err
	}
	stopExports, err := identityruntime.StartDirectoryExportLoop(ctx, dbConn.DB, cfg, storage, logger)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	stopReceipts, err := identityruntime.StartSessionReceiptCleanupLoop(ctx, identityapp.NewSessionReceiptCleanup(pgidentity.NewStore(dbConn.DB), time.Now), logger, 30*time.Second)
	if err != nil {
		stopExports()
		cleanup()
		return nil, nil, err
	}
	stopIdentity := func() { stopReceipts(); stopExports(); cleanup() }
	return &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}, stopIdentity, nil
}
