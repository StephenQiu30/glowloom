// Command gateway runs the independent public HTTP entry point.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	appgateway "github.com/StephenQiu30/glowloom-server/backend/internal/app/gateway"
	lvlog "github.com/StephenQiu30/glowloom-server/backend/internal/platform/log"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gateway:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := appgateway.LoadConfig()
	if err != nil {
		return err
	}
	logger, err := lvlog.New(cfg.Env, cfg.LogLevel)
	if err != nil {
		return err
	}
	defer func() { _ = logger.Sync() }()
	logger = logger.With(zap.String("service", "gateway"))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return appgateway.Run(ctx, cfg, logger)
}
