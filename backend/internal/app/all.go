package app

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
)

// RunAll serves the implemented API, flow worker, and relay roles together.
// A role failure cancels the others; the process waits for every role to exit.
func RunAll(ctx context.Context, cfg config.Config, logger *zap.Logger) error {
	roles := []struct {
		name string
		run  func(context.Context) error
	}{
		{"api", func(ctx context.Context) error { return RunAPI(ctx, cfg, logger) }},
		{"worker", func(ctx context.Context) error { return RunWorker(ctx, cfg, logger, "flow") }},
		{"relay", func(ctx context.Context) error { return RunRelay(ctx, cfg, logger) }},
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		name string
		err  error
	}
	results := make(chan result, len(roles))
	for _, role := range roles {
		go func() { results <- result{role.name, role.run(runCtx)} }()
	}
	first := <-results
	parentCancelled := ctx.Err() != nil
	cancel()
	var failure error
	for _, item := range []result{first, <-results, <-results} {
		if item.err != nil && !errors.Is(item.err, context.Canceled) && failure == nil {
			failure = fmt.Errorf("%s role: %w", item.name, item.err)
		}
	}
	if failure != nil {
		return failure
	}
	if parentCancelled {
		return nil
	}
	if first.err != nil {
		return fmt.Errorf("%s role: %w", first.name, first.err)
	}
	return fmt.Errorf("%s role stopped unexpectedly", first.name)
}
