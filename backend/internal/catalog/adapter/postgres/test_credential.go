package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// ErrTestSuperseded means a later test has already updated this credential.
var ErrTestSuperseded = errors.New("credential test superseded")

// LoadForCredentialTest rechecks administrator rights and reads the current
// sealed credential while holding provider and credential share locks.
func (s *Store) LoadForCredentialTest(ctx context.Context, request application.CredentialTestRequest) (domain.Provider, domain.Credential, error) {
	if s == nil || s.db == nil || request.ActorID == uuid.Nil || request.OrgID == uuid.Nil ||
		request.ProviderID == uuid.Nil || request.CredentialID == uuid.Nil {
		return domain.Provider{}, domain.Credential{}, application.ErrInvalidCredentialTest
	}
	var provider domain.Provider
	var credential domain.Credential
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentCatalogAdmin(tx, request.ActorID, request.OrgID); err != nil {
			return err
		}
		var p providerRow
		result := tx.Raw(`
			SELECT id, key, name, adapter_key, region, status, concurrency_limit,
			       rate_limit_per_min, revision, create_time, update_time
			FROM catalog.provider WHERE id = ?::uuid AND NOT is_delete FOR SHARE
		`, request.ProviderID.String()).Scan(&p)
		if result.Error != nil {
			return fmt.Errorf("read provider for credential test: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrProviderNotFound
		}
		if p.Status != string(domain.ProviderActive) {
			return ErrProviderUnavailable
		}
		var c credentialRow
		result = tx.Raw(`
			SELECT id, provider_id, label, ciphertext, key_id, last4, status,
			       last_tested_at, last_test_result, create_time, update_time
			FROM catalog.provider_credential
			WHERE id = ?::uuid AND provider_id = ?::uuid AND status = 'active'
			  AND NOT is_delete FOR SHARE
		`, request.CredentialID.String(), request.ProviderID.String()).Scan(&c)
		if result.Error != nil {
			return fmt.Errorf("read sealed credential for test: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrCredentialNotFound
		}
		provider, credential = p.provider(), c.credential()
		return nil
	})
	if err != nil {
		return domain.Provider{}, domain.Credential{}, fmt.Errorf("load credential test transaction: %w", err)
	}
	return provider, credential, nil
}

// RecordCredentialTest writes the result and both safe events in one
// transaction, after checking the same administrator and credential again.
func (s *Store) RecordCredentialTest(ctx context.Context, observation application.CredentialTestObservation, events []identityapp.OutboxEvent) error {
	if s == nil || s.db == nil || observation.TestID == uuid.Nil || observation.ActorID == uuid.Nil ||
		observation.OrgID == uuid.Nil || observation.ProviderID == uuid.Nil ||
		observation.CredentialID == uuid.Nil || observation.TestedAt.IsZero() ||
		!validCredentialTestEvents(observation, events) {
		return application.ErrInvalidCredentialTest
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentCatalogAdmin(tx, observation.ActorID, observation.OrgID); err != nil {
			return err
		}
		var provider struct{ Status string }
		result := tx.Raw(`
			SELECT status FROM catalog.provider WHERE id = ?::uuid AND NOT is_delete FOR UPDATE
		`, observation.ProviderID.String()).Scan(&provider)
		if result.Error != nil {
			return fmt.Errorf("lock provider for credential test: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrProviderNotFound
		}
		// The provider lock also serializes replay with another write of this test.
		var existing int
		result = tx.Raw(`SELECT 1 FROM infra.outbox WHERE id = ?::uuid`, events[0].ID.String()).Scan(&existing)
		if result.Error != nil {
			return fmt.Errorf("check credential test replay: %w", result.Error)
		}
		if result.RowsAffected == 1 {
			return nil
		}
		if provider.Status != string(domain.ProviderActive) {
			return ErrProviderUnavailable
		}
		var credential struct {
			Last4        string
			LastTestedAt *time.Time
		}
		result = tx.Raw(`
			SELECT last4, last_tested_at FROM catalog.provider_credential
			WHERE id = ?::uuid AND provider_id = ?::uuid AND status = 'active'
			  AND NOT is_delete FOR UPDATE
		`, observation.CredentialID.String(), observation.ProviderID.String()).Scan(&credential)
		if result.Error != nil {
			return fmt.Errorf("lock credential for test result: %w", result.Error)
		}
		if result.RowsAffected != 1 || credential.Last4 != observation.Last4 {
			return ErrCredentialNotFound
		}
		if credential.LastTestedAt != nil && !credential.LastTestedAt.Before(observation.TestedAt) {
			return ErrTestSuperseded
		}
		result = tx.Exec(`
			UPDATE catalog.provider_credential
			SET last_tested_at = ?, last_test_result = ?, update_time = now()
			WHERE id = ?::uuid AND provider_id = ?::uuid AND status = 'active' AND NOT is_delete
		`, observation.TestedAt.UTC(), string(observation.Result),
			observation.CredentialID.String(), observation.ProviderID.String())
		if result.Error != nil {
			return fmt.Errorf("update credential test result: %w", result.Error)
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
				return fmt.Errorf("insert credential test event %s: %w", event.Topic, result.Error)
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("insert credential test event %s: inserted %d rows", event.Topic, result.RowsAffected)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("record credential test transaction: %w", err)
	}
	return nil
}

type credentialTestChangedEvent struct {
	credentialChangedEvent
	Data struct {
		Change     string            `json:"change"`
		ProviderID uuid.UUID         `json:"provider_id"`
		Result     domain.TestResult `json:"result"`
	} `json:"data"`
}

func validCredentialTestEvents(observation application.CredentialTestObservation, events []identityapp.OutboxEvent) bool {
	if len(events) != 2 || events[0].ID != uuid.NewSHA1(observation.TestID, []byte("catalog.credential_changed.v1")) ||
		events[1].ID != uuid.NewSHA1(observation.TestID, []byte("audit.recorded.v1")) ||
		events[0].Topic != "lanverse.catalog.credential_changed.v1" || events[1].Topic != "lanverse.audit.recorded.v1" {
		return false
	}
	for _, event := range events {
		if event.PartitionKey != observation.OrgID.String() || len(event.Payload) == 0 || len(event.Payload) > 32*1024 {
			return false
		}
	}
	var changed credentialTestChangedEvent
	if !decodeCredentialEvent(events[0].Payload, &changed) || changed.EventID != events[0].ID ||
		changed.EventType != events[0].Topic || !changed.OccurredAt.Equal(observation.TestedAt.UTC()) ||
		changed.OrgID != observation.OrgID || changed.Actor.Kind != "user" ||
		changed.Actor.ID != observation.ActorID || changed.Aggregate.Type != "provider_credential" ||
		changed.Aggregate.ID != observation.CredentialID || changed.Data.Change != "tested" ||
		changed.Data.ProviderID != observation.ProviderID || changed.Data.Result != observation.Result {
		return false
	}
	var audit credentialAuditEvent
	return decodeCredentialEvent(events[1].Payload, &audit) && audit.EventID == events[1].ID &&
		audit.EventType == events[1].Topic && audit.OccurredAt.Equal(observation.TestedAt.UTC()) &&
		audit.OrgID == observation.OrgID && audit.Actor.Kind == "user" &&
		audit.Actor.ID == observation.ActorID && audit.Aggregate.Type == "audit" &&
		audit.Aggregate.ID == audit.EventID && audit.Data.Action == "credential.tested" &&
		audit.Data.Object.Type == "provider_credential" && audit.Data.Object.ID == observation.CredentialID &&
		audit.Data.After.Last4 == observation.Last4 && audit.Data.RequestID == observation.RequestID
}

var _ application.CredentialTestStore = (*Store)(nil)
