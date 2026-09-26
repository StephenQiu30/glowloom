package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/redisconn"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/temporalconn"
)

func provideDB(ctx context.Context, cfg config.Config, logger *zap.Logger) (*db.Connection, func(), error) {
	conn, err := db.Open(ctx, cfg.DBDSN)
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

func provideTemporal(cfg config.Config, logger *zap.Logger) (*temporalconn.Connection, func(), error) {
	conn, err := temporalconn.Open(cfg.TemporalAddr, cfg.TemporalNamespace, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("configure api Temporal: %w", err)
	}
	return conn, conn.Close, nil
}

func provideReadyCheck(dbConn *db.Connection, redisConn *redisconn.Connection, temporalConn *temporalconn.Connection) ReadyCheck {
	return func(ctx context.Context) error {
		if err := dbConn.Ping(ctx); err != nil {
			return err
		}
		if err := redisConn.Ping(ctx); err != nil {
			return err
		}
		return temporalConn.Ping(ctx)
	}
}

func provideAPIServer(cfg config.Config, logger *zap.Logger, ready ReadyCheck) *http.Server {
	return &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           NewRouter(logger, ready),
		ReadHeaderTimeout: 10 * time.Second,
	}
}
