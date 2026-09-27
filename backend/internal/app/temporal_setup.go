package app

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
)

// InstallCleanupSchedules installs the two implemented retention schedules.
// The namespace must already exist; prefix isolates verification schedules.
func InstallCleanupSchedules(ctx context.Context, cfg config.Config, logger *zap.Logger, prefix string) error {
	installer, cleanup, err := initializeMaintenanceSetup(ctx, cfg, logger, prefix)
	if err != nil {
		return fmt.Errorf("initialize Temporal setup: %w", err)
	}
	defer cleanup()
	if err := installer.InstallCleanupSchedules(ctx); err != nil {
		return fmt.Errorf("install cleanup schedules: %w", err)
	}
	logger.Info("cleanup schedules installed", zap.String("namespace", cfg.TemporalNamespace))
	return nil
}
