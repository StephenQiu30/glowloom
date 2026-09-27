package app

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
)

// RunWorker serves the implemented flow queue until the process context is cancelled.
func RunWorker(ctx context.Context, cfg config.Config, logger *zap.Logger, queues string) error {
	queue, err := WorkerQueue(queues)
	if err != nil {
		return err
	}
	flowWorker, cleanup, err := initializeWorker(ctx, cfg, logger, queue)
	if err != nil {
		return fmt.Errorf("initialize worker: %w", err)
	}
	defer cleanup()

	return RunWithHealth(ctx, "worker", cfg.WorkerHealthAddr, logger, func(runCtx context.Context) error {
		interrupt := make(chan any)
		finished := make(chan struct{})
		go func() {
			select {
			case <-runCtx.Done():
				close(interrupt)
			case <-finished:
			}
		}()
		logger.Info("worker polling", zap.String("queue", queue))
		err := flowWorker.Run(interrupt)
		close(finished)
		if err != nil {
			return fmt.Errorf("run worker queue %s: %w", queue, err)
		}
		return nil
	})
}
