package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.temporal.io/sdk/worker"

	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
)

// RunWorker serves each selected backend queue until cancellation or failure.
func RunWorker(ctx context.Context, cfg config.Config, logger *zap.Logger, queues string) error {
	selected, err := WorkerQueue(queues)
	if err != nil {
		return err
	}
	type readyWorker struct {
		queue   string
		worker  worker.Worker
		cleanup func()
	}
	var ready []readyWorker
	defer func() {
		for index := len(ready) - 1; index >= 0; index-- {
			ready[index].cleanup()
		}
	}()
	for _, queue := range strings.Split(selected, ",") {
		queueWorker, cleanup, err := initializeWorker(ctx, cfg, logger, queue)
		if err != nil {
			return fmt.Errorf("initialize %s worker: %w", queue, err)
		}
		ready = append(ready, readyWorker{queue: queue, worker: queueWorker, cleanup: cleanup})
	}

	return RunWithHealth(ctx, "worker", cfg.WorkerHealthAddr, logger, func(runCtx context.Context) error {
		ownedCtx, cancel := context.WithCancel(runCtx)
		defer cancel()
		type result struct {
			queue string
			err   error
		}
		results := make(chan result, len(ready))
		for _, item := range ready {
			go func() {
				interrupt := make(chan any)
				finished := make(chan struct{})
				go func() {
					select {
					case <-ownedCtx.Done():
						close(interrupt)
					case <-finished:
					}
				}()
				logger.Info("worker polling", zap.String("queue", item.queue))
				err := item.worker.Run(interrupt)
				close(finished)
				results <- result{queue: item.queue, err: err}
			}()
		}
		first := <-results
		cancel()
		failure := first
		for range len(ready) - 1 {
			item := <-results
			if (failure.err == nil || errors.Is(failure.err, context.Canceled)) && item.err != nil {
				failure = item
			}
		}
		if failure.err != nil && !errors.Is(failure.err, context.Canceled) {
			return fmt.Errorf("run worker queue %s: %w", failure.queue, failure.err)
		}
		if runCtx.Err() != nil {
			return nil
		}
		return fmt.Errorf("worker queue %s stopped unexpectedly", first.queue)
	})
}
