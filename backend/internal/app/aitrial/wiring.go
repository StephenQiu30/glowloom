// Package aitrial wires local administrator experiments to existing queues and storage.
package aitrial

import (
	"context"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"

	"github.com/gin-gonic/gin"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"gorm.io/gorm"

	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/credentialseal"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/codex"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/trial"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// RegisterAPI connects administrator routes to the existing local Temporal service.
func RegisterAPI(ctx context.Context, group *gin.RouterGroup, db *gorm.DB, objects *objectstorage.Client, cfg config.Config) error {
	var temporalClient client.Client
	if cfg.Env == "local" && cfg.TemporalAddr != "" && cfg.TemporalNamespace != "" {
		c, err := client.NewLazyClient(client.Options{HostPort: cfg.TemporalAddr, Namespace: cfg.TemporalNamespace})
		if err != nil {
			return err
		}
		temporalClient = c
		if ctx.Done() != nil {
			go func() { <-ctx.Done(); c.Close() }()
		}
	}
	operationhttp.NewTrialHandler(temporalClient, operationwire.NewCatalogStore(db), objects, application.TrialStorageID(cfg.ObjectStorageEndpoint, cfg.ObjectStorageBucket)).Register(group)
	return nil
}

// RegisterCloud registers cloud execution only on the provider worker.
func RegisterCloud(w worker.Worker, db *gorm.DB, keys *credentialseal.Unsealer, cloud *providerhttp.Client, objects *objectstorage.Client, storageID string) {
	runner := &trial.Runner{StorageID: storageID, Store: trial.NewStore(db, operationwire.NewCatalogStore(db)), Keys: keys, Cloud: cloud, Objects: objects}
	w.RegisterActivityWithOptions(runner.Run, activity.RegisterOptions{Name: "trial.Run"})
}

// RegisterCodex registers native execution only on the Codex worker.
func RegisterCodex(w worker.Worker, db *gorm.DB, launcher codex.Launcher, root string, objects *objectstorage.Client, storageID string) {
	runner := &trial.Runner{StorageID: storageID, Store: trial.NewStore(db, operationwire.NewCatalogStore(db)), Launcher: launcher, WorkRoot: root, Objects: objects}
	w.RegisterActivityWithOptions(runner.Run, activity.RegisterOptions{Name: "trial.Run"})
}
