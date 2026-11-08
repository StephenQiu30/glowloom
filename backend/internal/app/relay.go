package app

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	kafkainbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/adapter/kafka"
	outboxapp "github.com/StephenQiu30/glowloom-server/backend/internal/infra/outbox/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

const (
	realtimeConsumerGroup = "glowloom-realtime"
	auditConsumerGroup    = "glowloom-audit"
	creditConsumerGroup   = "glowloom-billing"
)

type relayRuntime struct {
	outbox   *outboxapp.Relay
	realtime *kafkainbox.Consumer
	audit    *kafkainbox.Consumer
	credit   *kafkainbox.Consumer
}

// RunRelay publishes Outbox rows, consumes projections, and starts confirmed workflows.
func RunRelay(ctx context.Context, cfg config.Config, logger *zap.Logger) error {
	runtime, cleanup, err := initializeRelay(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("initialize relay: %w", err)
	}
	defer cleanup()
	groups := []string{realtimeConsumerGroup, auditConsumerGroup, creditConsumerGroup}
	logger.Info("relay running", zap.Strings("consumer_groups", groups))
	return RunWithHealth(ctx, "relay", cfg.RelayHealthAddr, logger, func(runCtx context.Context) error {
		loops := []relayLoop{
			{"outbox", runtime.outbox.Run},
			{"realtime", runtime.realtime.Run},
			{"audit", runtime.audit.Run},
			{"credit", runtime.credit.Run},
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
