package postgres

import (
	"context"
	_ "embed"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

//go:embed storage_finalize.sql
var storageFinalizeSQL string

// FinalizeStorageSchema is an explicit maintenance operation. It retains owning
// table locks through proof, relationship replacement and constraint validation.
func (s *StorageRoutes) FinalizeStorageSchema(ctx context.Context, configSHA string) error {
	if s == nil || s.db == nil || !storageDigest(configSHA) {
		return application.ErrStorageNotReady
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL lock_timeout='5s'`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`SELECT set_config('glowloom.storage_config_sha256',?,true)`, configSHA).Error; err != nil {
			return err
		}
		var legacy bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.storage_legacy_binding)`).Scan(&legacy).Error; err != nil {
			return err
		}
		if !legacy {
			if err := requireNativeBootstrap(tx, configSHA); err != nil {
				return err
			}
		}
		if err := requireFrozenPendingWork(tx); err != nil {
			return err
		}
		// An already-finalized schema needs no DDL and may retain correctly pinned work.
		if err := NewStorageRoutes(tx).CheckStorageFinalSchema(ctx); err == nil {
			return nil
		} else if !errors.Is(err, application.ErrStorageNotReady) {
			return err
		}
		return tx.Exec(storageFinalizeSQL).Error
	})
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "P0001" && pg.Message == "storage finalization not ready" {
		return application.ErrStorageNotReady
	}
	return err
}
