package app

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/credentialseal"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/providerprobe"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/worker"
	"gorm.io/gorm"
)

func registerProviderActivities(w worker.Worker, database *gorm.DB, cfg config.Config) error {
	file, err := os.Open(cfg.CredentialPrivateKeyFile)
	if err != nil {
		return fmt.Errorf("provider worker requires readable private sealing key")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 16385))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(data) > 16384 {
		return fmt.Errorf("provider worker private key unreadable or oversized")
	}
	defer clear(data)
	unsealer, err := credentialseal.NewUnsealer(cfg.CredentialKeyID, data)
	if err != nil {
		return fmt.Errorf("configure provider worker sealing key: %w", err)
	}
	activities := providerprobe.NewActivities(database, unsealer, providerhttp.NewClient(&http.Client{}))
	w.RegisterActivityWithOptions(activities.TestCredential, activity.RegisterOptions{Name: "provider.test_credential"})
	return nil
}
