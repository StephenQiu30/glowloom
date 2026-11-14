// Command creation runs private document extraction without business data access.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/creationruntime"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "creation:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := creationruntime.LoadConfig()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return creationruntime.Run(ctx, cfg, logger)
}
