package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/redisconn"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/temporalconn"
)

const shutdownTimeout = 10 * time.Second

// RunAPI serves the api role until ctx is cancelled, then shuts down gracefully.
func RunAPI(ctx context.Context, cfg config.Config, logger *zap.Logger) error {
	conn, err := db.Open(ctx, cfg.DBDSN)
	if err != nil {
		return fmt.Errorf("connect api database: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			logger.Error("close api database", zap.Error(err))
		}
	}()
	redisconn.ConfigureLogging(logger)
	redisConn, err := redisconn.Open(cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("configure api Redis: %w", err)
	}
	defer func() {
		if err := redisConn.Close(); err != nil {
			logger.Error("close api Redis", zap.Error(err))
		}
	}()
	temporalConn, err := temporalconn.Open(cfg.TemporalAddr, cfg.TemporalNamespace, logger)
	if err != nil {
		return fmt.Errorf("configure api Temporal: %w", err)
	}
	defer temporalConn.Close()
	ready := func(ctx context.Context) error {
		if err := conn.Ping(ctx); err != nil {
			return err
		}
		if err := redisConn.Ping(ctx); err != nil {
			return err
		}
		return temporalConn.Ping(ctx)
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           NewRouter(logger, ready),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("api listening", zap.String("addr", cfg.HTTPAddr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("listen: %w", err)
		}
		close(errCh)
	}()

	select {
	case err, ok := <-errCh:
		if ok {
			return err
		}
		return nil
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return <-errCh
}
