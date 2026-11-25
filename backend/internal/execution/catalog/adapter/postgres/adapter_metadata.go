package postgres

import (
	"context"

	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// AuthorizeAdapterMetadata rechecks the current exact session and platform role.
func (s *Store) AuthorizeAdapterMetadata(ctx context.Context, actor identityapp.Principal) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return s.requirePlatformAdmin(tx, actor) })
}
