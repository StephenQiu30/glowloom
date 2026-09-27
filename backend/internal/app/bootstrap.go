package app

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	pgidentity "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

// BootstrapAdmin initializes the MVP organization and first administrator.
func BootstrapAdmin(ctx context.Context, cfg config.Config, logger *zap.Logger, input identityapp.BootstrapAdminInput) (identityapp.CreatedUser, error) {
	command, cleanup, err := initializeAdminBootstrap(ctx, cfg, logger)
	if err != nil {
		return identityapp.CreatedUser{}, fmt.Errorf("initialize administrator bootstrap: %w", err)
	}
	defer cleanup()
	created, err := command.Execute(ctx, input)
	if err != nil {
		return identityapp.CreatedUser{}, err
	}
	return created, nil
}

func provideBootstrapTrace(ctx context.Context, cfg config.Config, logger *zap.Logger) (trace.TracerProvider, func(), error) {
	return provideNamedTrace(ctx, cfg, logger, "admin-bootstrap")
}

func provideIdentityStore(conn *db.Connection) *pgidentity.Store {
	return pgidentity.NewStore(conn.DB)
}

func provideBootstrapAdminCommand(store *pgidentity.Store) *identityapp.BootstrapAdminCommand {
	return identityapp.NewBootstrapAdminCommand(store, time.Now)
}
