package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
)

const shutdownTimeout = 10 * time.Second

// RunAPI serves the api role until ctx is cancelled, then shuts down gracefully.
func RunAPI(ctx context.Context, cfg config.Config, logger *zap.Logger) error {
	srv, cleanup, err := initializeAPI(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("initialize api: %w", err)
	}
	defer cleanup()

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
