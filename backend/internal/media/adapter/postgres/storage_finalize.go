package postgres

import (
	"context"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// FinalizeStorageSchema checks that the migration pipeline recorded the final
// schema. Runtime code never executes schema DDL.
func (s *StorageRoutes) FinalizeStorageSchema(ctx context.Context, configSHA string) error {
	if s == nil || s.db == nil || !storageDigest(configSHA) {
		return application.ErrStorageNotReady
	}
	return s.CheckStorageFinalSchema(ctx)
}
