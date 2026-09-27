package postgres

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// FindCredentialForAdmin returns only safe metadata of the current credential.
func (s *Store) FindCredentialForAdmin(ctx context.Context, actorID, orgID, providerID, credentialID uuid.UUID) (application.CredentialToDisable, error) {
	if s == nil || s.db == nil || actorID == uuid.Nil || orgID == uuid.Nil ||
		providerID == uuid.Nil || credentialID == uuid.Nil {
		return application.CredentialToDisable{}, application.ErrInvalidDisableCredential
	}
	var target application.CredentialToDisable
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentCatalogAdmin(tx, actorID, orgID); err != nil {
			return err
		}
		var provider int
		result := tx.Raw(`
			SELECT 1 FROM catalog.provider WHERE id = ?::uuid AND NOT is_delete FOR SHARE
		`, providerID.String()).Scan(&provider)
		if result.Error != nil {
			return fmt.Errorf("read provider for credential disable: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrProviderNotFound
		}
		result = tx.Raw(`
			SELECT c.id, c.provider_id, c.last4
			FROM catalog.provider_credential AS c
			WHERE c.id = ?::uuid AND c.provider_id = ?::uuid
			  AND c.status = 'active' AND NOT c.is_delete
			FOR SHARE
		`, credentialID.String(), providerID.String()).Scan(&target)
		if result.Error != nil {
			return fmt.Errorf("read current credential metadata: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrCredentialNotFound
		}
		return nil
	})
	if err != nil {
		return application.CredentialToDisable{}, fmt.Errorf("find credential for administrator: %w", err)
	}
	return target, nil
}

// DisableCredentialWithEvents rechecks current access and target state, then
// commits the disable and its change/audit events as one transaction.
func (s *Store) DisableCredentialWithEvents(ctx context.Context, actorID, orgID uuid.UUID, target application.CredentialToDisable, events []identityapp.OutboxEvent) (application.SavedCredential, error) {
	if s == nil || s.db == nil || actorID == uuid.Nil || orgID == uuid.Nil ||
		target.ID == uuid.Nil || target.ProviderID == uuid.Nil ||
		!utf8.ValidString(target.Last4) || utf8.RuneCountInString(target.Last4) != 4 ||
		!validCredentialLifecycleEvents(actorID, orgID, target.ID, target.ProviderID,
			target.Last4, "disabled", "credential.disabled", events) {
		return application.SavedCredential{}, application.ErrInvalidDisableCredential
	}
	var saved application.SavedCredential
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentCatalogAdmin(tx, actorID, orgID); err != nil {
			return err
		}
		var provider int
		result := tx.Raw(`
			SELECT 1 FROM catalog.provider WHERE id = ?::uuid AND NOT is_delete FOR UPDATE
		`, target.ProviderID.String()).Scan(&provider)
		if result.Error != nil {
			return fmt.Errorf("lock provider for credential disable: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrProviderNotFound
		}
		var last4 string
		result = tx.Raw(`
			SELECT last4 FROM catalog.provider_credential
			WHERE id = ?::uuid AND provider_id = ?::uuid AND status = 'active'
			  AND NOT is_delete FOR UPDATE
		`, target.ID.String(), target.ProviderID.String()).Scan(&last4)
		if result.Error != nil {
			return fmt.Errorf("lock current credential for disable: %w", result.Error)
		}
		if result.RowsAffected != 1 || last4 != target.Last4 {
			return ErrCredentialNotFound
		}
		result = tx.Exec(`
			UPDATE catalog.provider_credential
			SET status = 'disabled', update_time = now()
			WHERE id = ?::uuid AND provider_id = ?::uuid AND status = 'active' AND NOT is_delete
		`, target.ID.String(), target.ProviderID.String())
		if result.Error != nil {
			return fmt.Errorf("disable current credential: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrCredentialNotFound
		}
		for _, event := range events {
			result = tx.Exec(`
				INSERT INTO infra.outbox (id, topic, partition_key, payload)
				VALUES (?::uuid, ?, ?, ?::jsonb)
			`, event.ID.String(), event.Topic, event.PartitionKey, string(event.Payload))
			if result.Error != nil {
				return fmt.Errorf("insert credential disable event %s: %w", event.Topic, result.Error)
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("insert credential disable event %s: inserted %d rows", event.Topic, result.RowsAffected)
			}
		}
		var row credentialRow
		result = tx.Raw(`
			SELECT id, provider_id, label, last4, status, last_tested_at,
			       last_test_result, create_time, update_time
			FROM catalog.provider_credential WHERE id = ?::uuid AND NOT is_delete
		`, target.ID.String()).Scan(&row)
		if result.Error != nil {
			return fmt.Errorf("read disabled credential summary: %w", result.Error)
		}
		if result.RowsAffected != 1 || row.Status != string(domain.CredentialDisabled) {
			return domain.ErrInvalidCredential
		}
		saved = application.SavedCredential{
			ID: row.ID, ProviderID: row.ProviderID, Label: row.Label,
			Last4: row.Last4, Status: domain.CredentialDisabled,
			CreateTime: row.CreateTime, UpdateTime: row.UpdateTime,
		}
		if row.LastTestResult != nil {
			value := domain.TestResult(*row.LastTestResult)
			saved.LastTestResult = &value
		}
		return nil
	})
	if err != nil {
		return application.SavedCredential{}, fmt.Errorf("disable credential transaction: %w", err)
	}
	return saved, nil
}

var _ application.DisableCredentialStore = (*Store)(nil)
