package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/bootstrap"
	"github.com/StephenQiu30/lanverse/backend/internal/config"
	"github.com/StephenQiu30/lanverse/backend/internal/database"
	"github.com/StephenQiu30/lanverse/backend/internal/telemetry"
)

const (
	shutdownTimeout  = 10 * time.Second
	migrationTimeout = 2 * time.Minute
)

var (
	buildVersion = "development"
	buildCommit  = "unknown"
	buildTime    = "unknown"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	configuration, err := config.Load()
	if err != nil {
		logger.Error("api configuration is invalid", "error", err)
		os.Exit(1)
	}
	databaseURL, err := database.MigrationURL()
	if err != nil {
		logger.Error("database migration configuration is invalid", "error", err)
		os.Exit(1)
	}
	migrationContext, cancelMigration := context.WithTimeout(
		context.Background(),
		migrationTimeout,
	)
	if err = database.ApplyMigrations(migrationContext, databaseURL); err != nil {
		cancelMigration()
		logger.Error("database migration failed", "error", err)
		os.Exit(1)
	}
	cancelMigration()
	logger.Info("database migrations applied")

	upstreamClient := &http.Client{Timeout: configuration.UpstreamTimeout}
	httpMetrics := telemetry.NewHTTPMetrics()
	server := &http.Server{
		Addr: configuration.ListenAddress,
		Handler: bootstrap.NewAPIHandler(
			configuration.LegacyAPIURL,
			upstreamClient,
			bootstrap.RuntimeOptions{
				Build: bootstrap.BuildInfo{
					Service: "lanverse-api",
					Version: buildVersion,
					Commit:  buildCommit,
					BuiltAt: buildTime,
				},
				Metrics: httpMetrics,
			},
		),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	shutdownSignal, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("lanverse api started", "address", configuration.ListenAddress)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case <-shutdownSignal.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			logger.Error("api shutdown failed", "error", err)
			os.Exit(1)
		}
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("api stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}
}
