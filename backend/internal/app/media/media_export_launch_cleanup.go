package media

import (
	"context"
	"time"

	"go.uber.org/zap"

	tool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
)

// StartExportLaunchCleanup uses the API lifetime and bounded receipt cleanup,
// independently of callers visiting a launch URL. Payloads/tokens are never logged.
func StartExportLaunchCleanup(ctx context.Context, store tool.LaunchStore, logger *zap.Logger) func() {
	runCtx, cancelRun := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case now := <-ticker.C:
				bounded, cancel := context.WithTimeout(runCtx, 5*time.Second)
				_, err := store.PruneLaunches(bounded, now.UTC(), 1000)
				if err != nil && runCtx.Err() == nil {
					logger.Warn("export launch receipt cleanup failed")
				}
				cancel()
			}
		}
	}()
	return func() { cancelRun(); <-done }
}
