// Command identity runs the independently owned Identity authentication process.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/StephenQiu30/glowloom-server/backend/identity/app"
)

func main() { os.Exit(run()) }
func run() int {
	logger, err := zap.NewProduction()
	if err != nil {
		return 1
	}
	defer func() { _ = logger.Sync() }()
	cfg, err := app.LoadEnvironment(os.LookupEnv)
	if err != nil {
		logger.Error("identity owner configuration rejected")
		return 1
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	startup, finishStartup := context.WithTimeout(ctx, 20*time.Second)
	runtime, err := app.New(startup, cfg, logger)
	finishStartup()
	if err != nil {
		logger.Error("identity authentication owner initialization failed")
		return 1
	}
	if err := runtime.Run(ctx); err != nil {
		logger.Error("identity authentication owner stopped unexpectedly")
		return 1
	}
	return 0
}
