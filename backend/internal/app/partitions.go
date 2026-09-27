package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	pgaudit "github.com/StephenQiu30/lanverse/backend/internal/audit/adapter/postgres"
	pgoutbox "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

// ErrInvalidPartitionMaintenanceRole means the database accounts cannot safely
// run privileged partition DDL.
var ErrInvalidPartitionMaintenanceRole = errors.New("invalid partition maintenance database roles")

type partitionDBIdentity struct {
	Role      string
	Database  string
	StartedAt time.Time
	Superuser bool
}

// EnsurePartitions prepares Outbox and audit months with an account separate
// from the application writer. Both tables change in one database transaction.
func EnsurePartitions(ctx context.Context, cfg config.Config, logger *zap.Logger, now time.Time) error {
	if strings.TrimSpace(cfg.PartitionMaintenanceDBDSN) == "" {
		return fmt.Errorf("%w: LV_PARTITION_MAINTENANCE_DB_DSN is required", ErrInvalidPartitionMaintenanceRole)
	}
	if strings.TrimSpace(cfg.DBDSN) == "" {
		return fmt.Errorf("%w: LV_DB_DSN is required", ErrInvalidPartitionMaintenanceRole)
	}
	provider, cleanup, err := provideNamedTrace(ctx, cfg, logger, "partition-maintenance")
	if err != nil {
		return fmt.Errorf("configure partition maintenance tracing: %w", err)
	}
	defer cleanup()

	appConn, err := db.Open(ctx, cfg.DBDSN, provider)
	if err != nil {
		return fmt.Errorf("connect application database for partition role check: %w", err)
	}
	defer func() {
		if err := appConn.Close(); err != nil {
			logger.Error("close application partition role check", zap.Error(err))
		}
	}()
	maintenanceConn, err := db.Open(ctx, cfg.PartitionMaintenanceDBDSN, provider)
	if err != nil {
		return fmt.Errorf("connect partition maintenance database: %w", err)
	}
	defer func() {
		if err := maintenanceConn.Close(); err != nil {
			logger.Error("close partition maintenance database", zap.Error(err))
		}
	}()

	if err := checkPartitionRoles(ctx, appConn.DB, maintenanceConn.DB); err != nil {
		return err
	}
	err = maintenanceConn.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := pgoutbox.NewPartitionStore(tx).EnsurePartitions(ctx, now); err != nil {
			return fmt.Errorf("prepare Outbox partitions: %w", err)
		}
		if err := pgaudit.NewPartitionStore(tx).EnsurePartitions(ctx, now); err != nil {
			return fmt.Errorf("prepare audit partitions: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("ensure writable partitions: %w", err)
	}
	return nil
}

func checkPartitionRoles(ctx context.Context, appDB, maintenanceDB *gorm.DB) error {
	appRole, err := readPartitionIdentity(ctx, appDB)
	if err != nil {
		return fmt.Errorf("identify application database role: %w", err)
	}
	maintenanceRole, err := readPartitionIdentity(ctx, maintenanceDB)
	if err != nil {
		return fmt.Errorf("identify maintenance database role: %w", err)
	}
	if appRole.Database != maintenanceRole.Database || !appRole.StartedAt.Equal(maintenanceRole.StartedAt) {
		return fmt.Errorf("%w: application and maintenance connections must reach the same database", ErrInvalidPartitionMaintenanceRole)
	}
	if appRole.Role == maintenanceRole.Role || appRole.Superuser {
		return fmt.Errorf("%w: application account must be a separate non-superuser", ErrInvalidPartitionMaintenanceRole)
	}
	for _, table := range []string{"infra.outbox", "audit.audit_log"} {
		var ownership struct {
			Owner       string
			AppIsMember bool
		}
		if err := maintenanceDB.WithContext(ctx).Raw(`
			SELECT pg_get_userbyid(c.relowner) AS owner,
			       pg_has_role(?::name, c.relowner, 'MEMBER') AS app_is_member
			FROM pg_class AS c WHERE c.oid = ?::regclass
		`, appRole.Role, table).Scan(&ownership).Error; err != nil {
			return fmt.Errorf("check %s partition owner: %w", table, err)
		}
		if ownership.Owner != maintenanceRole.Role || ownership.AppIsMember {
			return fmt.Errorf("%w: %s must be owned only by the maintenance account", ErrInvalidPartitionMaintenanceRole, table)
		}
	}
	return nil
}

func readPartitionIdentity(ctx context.Context, handle *gorm.DB) (partitionDBIdentity, error) {
	var identity partitionDBIdentity
	if err := handle.WithContext(ctx).Raw(`
		SELECT current_user AS role, current_database() AS database,
		       pg_postmaster_start_time() AS started_at, r.rolsuper AS superuser
		FROM pg_roles AS r WHERE r.rolname = current_user
	`).Scan(&identity).Error; err != nil {
		return partitionDBIdentity{}, err
	}
	if identity.Role == "" || identity.Database == "" || identity.StartedAt.IsZero() {
		return partitionDBIdentity{}, fmt.Errorf("database role identity is incomplete")
	}
	return identity, nil
}
