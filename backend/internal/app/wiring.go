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
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/otelconn"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/redisconn"
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

func provideObjectStorage(cfg config.Config) (*objectstorage.Registry, func(), error) {
	if cfg.ObjectStorage == nil {
		return nil, nil, fmt.Errorf("configure api object storage: %w", objectstorage.ErrUnavailable)
	}
	conn, err := objectstorage.OpenProfiles(*cfg.ObjectStorage)
	if err != nil {
		return nil, nil, fmt.Errorf("configure api object storage: %w", err)
	}
	return conn, conn.Close, nil
}

func provideReadyCheck(cfg config.Config, dbConn *db.Connection, redisConn *redisconn.Connection, storage *objectstorage.Registry) ReadyCheck {
	return func(ctx context.Context) error {
		if err := dbConn.Ping(ctx); err != nil {
			return err
		}
		if err := redisConn.Ping(ctx); err != nil {
			return err
		}
		if cfg.ObjectStorage == nil || storage == nil {
			return mediaapp.ErrStorageNotReady
		}
		profile, ok := cfg.ObjectStorage.Profiles[cfg.ObjectStorage.DefaultWriteProfile]
		if !ok {
			return mediaapp.ErrStorageNotReady
		}
		route, err := pgmedia.NewStorageRoutes(dbConn.DB).ReadConfigured(ctx)
		if err != nil {
			return err
		}
		if route.ProfileID != cfg.ObjectStorage.DefaultWriteProfile || route.Bucket != profile.Bucket || route.Revision != cfg.ObjectStorage.Revision {
			return mediaapp.ErrStorageRouteConflict
		}
		return storage.Ping(ctx)
	}
}

func provideAPIServer(ctx context.Context, cfg config.Config, logger *zap.Logger, ready ReadyCheck, tracerProvider trace.TracerProvider, dbConn *db.Connection, storage *objectstorage.Registry, redisConn *redisconn.Connection) (*http.Server, func(), error) {
	sessions, err := identityruntime.NewSessionDependencies(cfg, redisConn.Client)
	if err != nil {
		return nil, nil, err
	}
	router, err := NewBusinessRouter(logger, ready, tracerProvider, cfg, dbConn.DB, storage, sessions, ctx)
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
	stopStorageRuntime, err := startStorageRuntime(ctx, cfg, true, dbConn.DB)
	if err != nil {
		stopReceipts()
		stopExports()
		cleanup()
		return nil, nil, err
	}
	stopIdentity := func() { stopStorageRuntime(); stopReceipts(); stopExports(); cleanup() }
	return &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}, stopIdentity, nil
}
