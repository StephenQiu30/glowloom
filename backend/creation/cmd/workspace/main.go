// Command workspace runs Creation's actual stateful project/source API and workers.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/app"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "creation workspace:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := workspaceapp.LoadEnvironment(os.LookupEnv)
	if err != nil {
		return err
	}
	logger, err := zap.NewProduction()
	if err != nil {
		return err
	}
	defer func() { _ = logger.Sync() }()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runtime, err := workspaceapp.New(ctx, cfg, logger)
	if err != nil {
		return err
	}
	return runtime.Run(ctx)
}
