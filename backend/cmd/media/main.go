// Command media runs private Media computation without business data access.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/mediaruntime"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "media:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := mediaruntime.LoadConfig()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "media")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return mediaruntime.Run(ctx, cfg, logger)
}
