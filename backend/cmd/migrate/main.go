package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/StephenQiu30/lanverse/backend/internal/database"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	databaseURL, err := database.MigrationURL()
	if err != nil {
		logger.Error("migration configuration is invalid", "error", err)
		os.Exit(1)
	}
	if err = database.ApplyMigrations(context.Background(), databaseURL); err != nil {
		logger.Error("database migration failed", "error", err)
		os.Exit(1)
	}
	logger.Info("database migrations are current")
}
