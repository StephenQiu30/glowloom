package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/bootstrap"
	"github.com/StephenQiu30/lanverse/backend/internal/telemetry"
)

const (
	backendStartupTimeout = 2 * time.Minute
	runtimeRetryDelay     = 5 * time.Second
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "lanverse backend stopped: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	logger, closeLogger, err := telemetry.NewLogstashLogger(
		os.Stdout, "lanverse-backend", os.Getenv("ENVIRONMENT"), os.Getenv("LOGSTASH_ADDRESS"),
	)
	if err != nil {
		return fmt.Errorf("configure logging: %w", err)
	}
	defer func() { _ = closeLogger.Close() }()
	runtimeContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if len(os.Args) > 1 {
		return bootstrap.RunEventWorker(runtimeContext, logger)
	}

	apiStopped := make(chan error, 1)
	go func() {
		apiStopped <- bootstrap.RunAPI(runtimeContext, logger)
	}()
	if err = waitForAPI(apiStopped); err != nil {
		return fmt.Errorf("start API runtime: %w", err)
	}
	go superviseRuntime(runtimeContext, logger, "workflow", func(ctx context.Context) error {
		return bootstrap.RunWorkflowWorker(ctx, logger)
	})
	go superviseRuntime(runtimeContext, logger, "event", func(ctx context.Context) error {
		return bootstrap.RunEventWorker(ctx, logger)
	})
	if err = <-apiStopped; err != nil {
		return fmt.Errorf("API runtime stopped: %w", err)
	}
	return nil
}

func superviseRuntime(
	ctx context.Context,
	logger *slog.Logger,
	name string,
	runRuntime func(context.Context) error,
) {
	for ctx.Err() == nil {
		err := runRuntime(ctx)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			err = fmt.Errorf("runtime stopped unexpectedly")
		}
		logger.Error("backend runtime stopped; retrying", "runtime", name, "error", err,
			"retry_delay_ms", runtimeRetryDelay.Milliseconds())
		timer := time.NewTimer(runtimeRetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func waitForAPI(apiStopped <-chan error) error {
	deadline := time.NewTimer(backendStartupTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: time.Second}
	for {
		select {
		case err := <-apiStopped:
			if err == nil {
				return fmt.Errorf("API stopped before readiness")
			}
			return fmt.Errorf("API stopped before readiness: %w", err)
		case <-deadline.C:
			return fmt.Errorf("API readiness timed out")
		case <-ticker.C:
			response, err := client.Get(apiReadinessURL())
			if err == nil {
				_ = response.Body.Close()
				if response.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
	}
}

func apiReadinessURL() string {
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8686"
	}
	return "http://" + net.JoinHostPort("127.0.0.1", port) + "/readyz"
}
