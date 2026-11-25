package app

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

// RunAll runs the modular monolith: HTTP API, background workers, and Kafka event relay
// under one errgroup lifecycle. Any role failure or unexpected exit cancels siblings
// and waits for every goroutine to finish cleanly.
func RunAll(ctx context.Context, cfg config.Config, logger *zap.Logger) error {
	roles := []struct {
		name string
		run  func(context.Context) error
	}{
		{"api", func(ctx context.Context) error { return RunAPI(ctx, cfg, logger) }},
		{"worker", func(ctx context.Context) error { return RunWorker(ctx, cfg, logger, "auto") }},
		{"relay", func(ctx context.Context) error { return RunRelay(ctx, cfg, logger) }},
	}

	group, groupCtx := errgroup.WithContext(ctx)
	for _, role := range roles {
		group.Go(func() error {
			err := role.run(groupCtx)
			if err != nil {
				if errors.Is(err, context.Canceled) && ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("%s role: %w", role.name, err)
			}
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("%s role stopped unexpectedly", role.name)
		})
	}
	return group.Wait()
}
