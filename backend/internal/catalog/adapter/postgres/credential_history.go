package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ListCredentialHistory lists bounded metadata without selecting ciphertext or fingerprints.
func (s *Store) ListCredentialHistory(ctx context.Context, actor identityapp.Principal, id uuid.UUID) ([]application.ProviderCredentialSummary, error) {
	items := []application.ProviderCredentialSummary{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requirePlatformCatalogAdmin(tx, actor); err != nil {
			return err
		}
		var rows []credentialDisplayRow
		if err := tx.Raw(`SELECT id,provider_id,label,last4,status,last_tested_at,last_test_result,create_time,update_time FROM catalog.provider_credential WHERE provider_id=? AND NOT is_delete ORDER BY create_time DESC,id LIMIT 100`, id).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			items = append(items, *row.summary())
		}
		return nil
	})
	return items, err
}
