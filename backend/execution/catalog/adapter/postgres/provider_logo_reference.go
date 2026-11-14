package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProviderLogoReferences retains bound display assets independently of execution.
type ProviderLogoReferences struct{ db *gorm.DB }

// NewProviderLogoReferences injects the caller's media transaction.
func NewProviderLogoReferences(db *gorm.DB) *ProviderLogoReferences {
	return &ProviderLogoReferences{db: db}
}

// HasRetainedMediaReference retains display assets while a live provider binds them.
func (s *ProviderLogoReferences) HasRetainedMediaReference(ctx context.Context, id uuid.UUID) (bool, error) {
	var retained bool
	err := s.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM catalog.provider WHERE logo_asset_id=? AND NOT is_delete)`, id).Row().Scan(&retained)
	return retained, err
}
