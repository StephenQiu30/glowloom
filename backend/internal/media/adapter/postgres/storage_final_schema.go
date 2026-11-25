package postgres

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// CheckStorageFinalSchema verifies the storage schema version recorded by db/schema.sql.
func (s *StorageRoutes) CheckStorageFinalSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	var version storageSchemaVersionRecord
	err := s.db.WithContext(ctx).Where("version = ?", storageSchemaVersionCurrent).Take(&version).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrStorageNotReady
	}
	return err
}
