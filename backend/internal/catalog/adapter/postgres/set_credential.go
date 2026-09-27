package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// FindProviderForAdmin checks current account rights before exposing provider settings.
func (s *Store) FindProviderForAdmin(ctx context.Context, actorID, orgID, providerID uuid.UUID) (domain.Provider, error) {
	if s == nil || s.db == nil || actorID == uuid.Nil || orgID == uuid.Nil || providerID == uuid.Nil {
		return domain.Provider{}, identityapp.ErrForbidden
	}
	var provider domain.Provider
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentCatalogAdmin(tx, actorID, orgID); err != nil {
			return err
		}
		var err error
		provider, err = NewStore(tx).FindProvider(ctx, providerID)
		return err
	})
	if err != nil {
		return domain.Provider{}, fmt.Errorf("read provider as administrator: %w", err)
	}
	return provider, nil
}

// ReplaceWithEvents rechecks administrator rights and saves a sealed credential
// together with its cache-invalidation and audit events in one transaction.
func (s *Store) ReplaceWithEvents(ctx context.Context, actorID, orgID uuid.UUID, credential domain.Credential, events []identityapp.OutboxEvent) (domain.Credential, error) {
	if s == nil || s.db == nil || actorID == uuid.Nil || orgID == uuid.Nil ||
		credential.Validate() != nil || credential.Status != domain.CredentialActive ||
		credential.LastTestResult != "" || !validCredentialEvents(actorID, orgID, credential, events) {
		return domain.Credential{}, domain.ErrInvalidCredential
	}
	var saved domain.Credential
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentCatalogAdmin(tx, actorID, orgID); err != nil {
			return err
		}
		if err := replaceCredentialInTx(tx, credential); err != nil {
			return err
		}
		for _, event := range events {
			result := tx.Exec(`
				INSERT INTO infra.outbox (id, topic, partition_key, payload)
				VALUES (?::uuid, ?, ?, ?::jsonb)
			`, event.ID.String(), event.Topic, event.PartitionKey, string(event.Payload))
			if result.Error != nil {
				return fmt.Errorf("insert credential outbox event %s: %w", event.Topic, result.Error)
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("insert credential outbox event %s: inserted %d rows", event.Topic, result.RowsAffected)
			}
		}
		var row credentialRow
		result := tx.Raw(`
			SELECT id, provider_id, label, ciphertext, key_id, last4,
			       status, last_tested_at, last_test_result, create_time, update_time
			FROM catalog.provider_credential WHERE id = ?::uuid AND NOT is_delete
		`, credential.ID.String()).Scan(&row)
		if result.Error != nil {
			return fmt.Errorf("read saved credential: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return domain.ErrInvalidCredential
		}
		saved = row.credential()
		return nil
	})
	if err != nil {
		return domain.Credential{}, fmt.Errorf("replace credential transaction: %w", err)
	}
	return saved, nil
}

func requireCurrentCatalogAdmin(tx *gorm.DB, actorID, orgID uuid.UUID) error {
	var actor struct {
		Role               string
		Status             string
		MustChangePassword bool
	}
	result := tx.Raw(`
		SELECT role, status, must_change_password FROM identity."user"
		WHERE org_id = ?::uuid AND id = ?::uuid AND NOT is_delete FOR SHARE
	`, orgID.String(), actorID.String()).Scan(&actor)
	if result.Error != nil {
		return fmt.Errorf("read current catalog administrator: %w", result.Error)
	}
	if result.RowsAffected != 1 || actor.Role != string(identitydomain.RoleAdmin) ||
		actor.Status != string(identitydomain.StatusActive) || actor.MustChangePassword {
		return identityapp.ErrForbidden
	}
	return nil
}

type credentialChangedEvent struct {
	EventID    uuid.UUID `json:"event_id"`
	EventType  string    `json:"event_type"`
	OccurredAt time.Time `json:"occurred_at"`
	OrgID      uuid.UUID `json:"org_id"`
	Actor      struct {
		Kind string    `json:"kind"`
		ID   uuid.UUID `json:"id"`
	} `json:"actor"`
	Aggregate struct {
		Type string    `json:"type"`
		ID   uuid.UUID `json:"id"`
	} `json:"aggregate"`
	Data struct {
		Change     string    `json:"change"`
		ProviderID uuid.UUID `json:"provider_id"`
	} `json:"data"`
}

type credentialAuditEvent struct {
	EventID    uuid.UUID `json:"event_id"`
	EventType  string    `json:"event_type"`
	OccurredAt time.Time `json:"occurred_at"`
	OrgID      uuid.UUID `json:"org_id"`
	Actor      struct {
		Kind string    `json:"kind"`
		ID   uuid.UUID `json:"id"`
	} `json:"actor"`
	Aggregate struct {
		Type string    `json:"type"`
		ID   uuid.UUID `json:"id"`
	} `json:"aggregate"`
	Data struct {
		Action string `json:"action"`
		Object struct {
			Type string    `json:"type"`
			ID   uuid.UUID `json:"id"`
		} `json:"object"`
		After struct {
			Last4 string `json:"last4"`
		} `json:"after"`
		RequestID string `json:"request_id"`
	} `json:"data"`
}

func validCredentialEvents(actorID, orgID uuid.UUID, credential domain.Credential, events []identityapp.OutboxEvent) bool {
	return validCredentialLifecycleEvents(actorID, orgID, credential.ID, credential.ProviderID,
		credential.Last4, "set", "credential.set", events)
}

func validCredentialLifecycleEvents(actorID, orgID, credentialID, providerID uuid.UUID, last4, change, action string, events []identityapp.OutboxEvent) bool {
	if len(events) != 2 || events[0].ID == uuid.Nil || events[1].ID == uuid.Nil || events[0].ID == events[1].ID ||
		events[0].Topic != "lanverse.catalog.credential_changed.v1" || events[1].Topic != "lanverse.audit.recorded.v1" {
		return false
	}
	for _, event := range events {
		if event.PartitionKey != orgID.String() || len(event.Payload) == 0 || len(event.Payload) > 32*1024 {
			return false
		}
	}
	var changed credentialChangedEvent
	if !decodeCredentialEvent(events[0].Payload, &changed) || changed.EventID != events[0].ID ||
		changed.EventType != events[0].Topic || changed.OccurredAt.IsZero() || changed.OrgID != orgID ||
		changed.Actor.Kind != "user" || changed.Actor.ID != actorID ||
		changed.Aggregate.Type != "provider_credential" || changed.Aggregate.ID != credentialID ||
		changed.Data.Change != change || changed.Data.ProviderID != providerID {
		return false
	}
	var audit credentialAuditEvent
	if !decodeCredentialEvent(events[1].Payload, &audit) {
		return false
	}
	requestID, err := uuid.Parse(audit.Data.RequestID)
	return err == nil && requestID.String() == audit.Data.RequestID && audit.EventID == events[1].ID &&
		audit.EventType == events[1].Topic && audit.OccurredAt.Equal(changed.OccurredAt) && audit.OrgID == orgID &&
		audit.Actor.Kind == "user" && audit.Actor.ID == actorID &&
		audit.Aggregate.Type == "audit" && audit.Aggregate.ID == audit.EventID &&
		audit.Data.Action == action && audit.Data.Object.Type == "provider_credential" &&
		audit.Data.Object.ID == credentialID && audit.Data.After.Last4 == last4
}

func decodeCredentialEvent[T any](payload json.RawMessage, target *T) bool {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return false
	}
	var trailing any
	return decoder.Decode(&trailing) == io.EOF
}

var _ application.SetCredentialStore = (*Store)(nil)
