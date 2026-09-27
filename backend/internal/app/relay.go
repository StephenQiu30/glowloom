package app

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	kafkainbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/kafka"
	outboxapp "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
)

const (
	realtimeConsumerGroup = "lanverse-realtime"
	auditConsumerGroup    = "lanverse-audit"
)

type relayRuntime struct {
	outbox   *outboxapp.Relay
	realtime *kafkainbox.Consumer
	audit    *kafkainbox.Consumer
}

// RunRelay publishes committed Outbox rows and projects supported realtime events.
func RunRelay(ctx context.Context, cfg config.Config, logger *zap.Logger) error {
	runtime, cleanup, err := initializeRelay(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("initialize relay: %w", err)
	}
	defer cleanup()
	logger.Info("relay running", zap.Strings("consumer_groups", []string{realtimeConsumerGroup, auditConsumerGroup}))
	return RunWithHealth(ctx, "relay", cfg.RelayHealthAddr, logger, func(runCtx context.Context) error {
		return runRelayLoops(runCtx, runtime.outbox.Run, runtime.realtime.Run, runtime.audit.Run)
	})
}

func runRelayLoops(ctx context.Context, publish, realtime, audit func(context.Context) error) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		name string
		err  error
	}
	results := make(chan result, 3)
	go func() { results <- result{"outbox", publish(runCtx)} }()
	go func() { results <- result{"realtime", realtime(runCtx)} }()
	go func() { results <- result{"audit", audit(runCtx)} }()
	first := <-results
	parentCancelled := ctx.Err() != nil
	cancel()
	var failure error
	for _, item := range []result{first, <-results, <-results} {
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
