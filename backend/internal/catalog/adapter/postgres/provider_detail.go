package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// FindProviderDetailForAdmin reads a provider and only the current credential's
// display columns after checking live administrator rights in one transaction.
func (s *Store) FindProviderDetailForAdmin(ctx context.Context, actorID, orgID, providerID uuid.UUID) (domain.Provider, *application.ProviderCredentialSummary, error) {
	if s == nil || s.db == nil || actorID == uuid.Nil || orgID == uuid.Nil || providerID == uuid.Nil {
		return domain.Provider{}, nil, identityapp.ErrForbidden
	}
	var provider domain.Provider
	var credential *application.ProviderCredentialSummary
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentCatalogAdmin(tx, actorID, orgID); err != nil {
			return err
		}
		var providerData providerRow
		result := tx.Raw(`
			SELECT id, key, name, adapter_key, region, status, concurrency_limit,
			       rate_limit_per_min, revision, create_time, update_time
			FROM catalog.provider WHERE id = ?::uuid AND NOT is_delete FOR SHARE
		`, providerID.String()).Scan(&providerData)
		if result.Error != nil {
			return fmt.Errorf("read provider detail: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrProviderNotFound
		}
		provider = providerData.provider()
		var credentialData credentialDisplayRow
		result = tx.Raw(`
			SELECT id, provider_id, label, last4, status, last_tested_at,
			       last_test_result, create_time, update_time
			FROM catalog.provider_credential
			WHERE provider_id = ?::uuid AND status = 'active' AND NOT is_delete
			FOR SHARE
		`, providerID.String()).Scan(&credentialData)
		if result.Error != nil {
			return fmt.Errorf("read credential display metadata: %w", result.Error)
		}
		if result.RowsAffected == 1 {
			credential = credentialData.summary()
		}
		return nil
	})
	if err != nil {
		return domain.Provider{}, nil, fmt.Errorf("read provider detail as administrator: %w", err)
	}
	return provider, credential, nil
}

type credentialDisplayRow struct {
	ID             uuid.UUID
	ProviderID     uuid.UUID
	Label          string
	Last4          string
	Status         string
	LastTestedAt   *time.Time
	LastTestResult *string
	CreateTime     time.Time
	UpdateTime     time.Time
}

func (row credentialDisplayRow) summary() *application.ProviderCredentialSummary {
	summary := &application.ProviderCredentialSummary{
		ID: row.ID, ProviderID: row.ProviderID, Label: row.Label, Last4: row.Last4,
		Status: domain.CredentialStatus(row.Status), LastTestedAt: row.LastTestedAt,
		CreateTime: row.CreateTime, UpdateTime: row.UpdateTime,
	}
	if row.LastTestResult != nil {
		result := domain.TestResult(*row.LastTestResult)
		summary.LastTestResult = &result
	}
	return summary
}

var _ application.ProviderDetailStore = (*Store)(nil)
