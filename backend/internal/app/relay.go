package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	kafkainbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/kafka"
	outboxapp "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
)

const (
	realtimeConsumerGroup = "lanverse-realtime"
	auditConsumerGroup    = "lanverse-audit"
	workflowStarterGroup  = "lanverse-workflow-starter"
)

type relayRuntime struct {
	outbox   *outboxapp.Relay
	realtime *kafkainbox.Consumer
	audit    *kafkainbox.Consumer
	starter  *kafkainbox.Consumer
	sweep    func(context.Context) error
}

// RunRelay publishes Outbox rows, consumes projections, and starts confirmed workflows.
func RunRelay(ctx context.Context, cfg config.Config, logger *zap.Logger) error {
	runtime, cleanup, err := initializeRelay(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("initialize relay: %w", err)
	}
	defer cleanup()
	groups := []string{realtimeConsumerGroup, auditConsumerGroup}
	if runtime.starter != nil {
		groups = append(groups, workflowStarterGroup)
	} else {
		logger.Warn("workflow starter disabled: Temporal is not configured")
	}
	logger.Info("relay running", zap.Strings("consumer_groups", groups))
	return RunWithHealth(ctx, "relay", cfg.RelayHealthAddr, logger, func(runCtx context.Context) error {
		loops := []relayLoop{
			{"outbox", runtime.outbox.Run},
			{"realtime", runtime.realtime.Run},
			{"audit", runtime.audit.Run},
		}
		if runtime.starter != nil {
			loops = append(loops,
				relayLoop{"workflow-starter", runtime.starter.Run},
				relayLoop{"workflow-starter-sweep", runtime.sweep},
			)
		}
		return runRelayLoops(runCtx, loops)
	})
}

type relayLoop struct {
	name string
	run  func(context.Context) error
}

func runRelayLoops(ctx context.Context, loops []relayLoop) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan relayResult, len(loops))
	for _, loop := range loops {
		go func() { results <- relayResult{loop.name, loop.run(runCtx)} }()
	}
	first := <-results
	parentCancelled := ctx.Err() != nil
	cancel()
	var failure error
	for i := 0; i < len(loops); i++ {
		item := first
		if i > 0 {
			item = <-results
		}
		if item.err != nil && !errors.Is(item.err, context.Canceled) && failure == nil {
			failure = fmt.Errorf("%s relay loop: %w", item.name, item.err)
		}
	}
	if failure != nil {
		return failure
	}
	if parentCancelled {
		return nil
	}
	if first.err != nil {
		return fmt.Errorf("%s relay loop: %w", first.name, first.err)
	}
	return fmt.Errorf("%s relay loop stopped unexpectedly", first.name)
}

type relayResult struct {
	name string
	err  error
}

func runStarterSweep(ctx context.Context, sweep func(context.Context, time.Time) error) error {
	if err := sweep(ctx, time.Now()); err != nil {
		return err
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			if err := sweep(ctx, now); err != nil {
				return err
			}
		}
	}
}
