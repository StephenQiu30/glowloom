// Package aitrial wires local administrator experiments to existing queues and storage.
package aitrial

import (
	"context"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/catalog/adapter/credentialseal"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/codex"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/providerhttp"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/trial"
	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// RegisterAPI exposes the database-backed local development trial service.
func RegisterAPI(_ context.Context, group *gin.RouterGroup, db *gorm.DB, registry *objectstorage.Registry, cfg config.Config) error {
	var sessions operationhttp.TrialClient
	if cfg.Env == "local" {
		sessions = Sessions(db, registry)
	}
	operationhttp.NewTrialHandler(sessions, operationwire.NewCatalogStore(db), NewObjects(registry)).Register(group)
	return nil
}

// Cloud wires the configured cloud trial executor.
func Cloud(db *gorm.DB, keys *credentialseal.Unsealer, cloud *providerhttp.Client, registry *objectstorage.Registry) taskruntime.Handler {
	return Sessions(db, registry).Handler(&trial.Runner{Store: trial.NewStore(db, operationwire.NewCatalogStore(db)), Keys: keys, Cloud: cloud, Objects: NewObjects(registry), Generated: operationwire.NewGeneratedVideos()})
}

// Codex wires the configured Codex trial executor.
func Codex(db *gorm.DB, launcher codex.Launcher, root string, registry *objectstorage.Registry) taskruntime.Handler {
	return Sessions(db, registry).Handler(&trial.Runner{Store: trial.NewStore(db, operationwire.NewCatalogStore(db)), Launcher: launcher, WorkRoot: root, Objects: NewObjects(registry)})
}
