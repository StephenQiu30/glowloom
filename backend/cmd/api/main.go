package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/stephenqiu30/lanverse/backend/internal/modules/scripts/adapters/postgres"
	"github.com/stephenqiu30/lanverse/backend/internal/modules/scripts/application"
	httptransport "github.com/stephenqiu30/lanverse/backend/internal/modules/scripts/transport/http"
	"github.com/stephenqiu30/lanverse/backend/internal/platform/database"
	"github.com/stephenqiu30/lanverse/backend/internal/platform/objectstorage"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx)
	if err != nil {
		logFatal("database connection failed", err)
	}
	defer pool.Close()
	storage, err := objectstorage.New(ctx)
	if err != nil {
		logFatal("object storage connection failed", err)
	}
	service := application.NewService(postgres.New(pool, storage))
	server := &http.Server{
		Addr:              envOr("API_ADDR", "127.0.0.1:8686"),
		Handler:           httptransport.NewHandler(service).Router(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	slog.Info("lanverse api listening", "addr", server.Addr, "storage_bucket", storage.Bucket())
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logFatal("api server failed", err)
	}
}

func logFatal(message string, err error) {
	slog.Error(message, "error", err)
	os.Exit(1)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
