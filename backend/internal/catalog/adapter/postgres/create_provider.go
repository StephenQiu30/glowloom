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

// CreateProviderWithAudit rechecks the live administrator and commits the
// provider registration and its audit event in one transaction.
func (s *Store) CreateProviderWithAudit(ctx context.Context, actorID, orgID uuid.UUID, provider domain.Provider, event identityapp.OutboxEvent) (domain.Provider, error) {
	if s == nil || s.db == nil || actorID == uuid.Nil || orgID == uuid.Nil ||
		provider.Validate() != nil || provider.Revision != 1 || provider.Status != domain.ProviderActive ||
		!validProviderCreatedAudit(actorID, orgID, provider, event) {
		return domain.Provider{}, application.ErrInvalidCreateProvider
	}
	var saved domain.Provider
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentCatalogAdmin(tx, actorID, orgID); err != nil {
			return err
		}
		if err := NewStore(tx).CreateProvider(ctx, provider); err != nil {
			return err
		}
		result := tx.Exec(`
			INSERT INTO infra.outbox (id, topic, partition_key, payload)
			VALUES (?::uuid, ?, ?, ?::jsonb)
		`, event.ID.String(), event.Topic, event.PartitionKey, string(event.Payload))
		if result.Error != nil {
			return fmt.Errorf("insert provider audit event: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("insert provider audit event: inserted %d rows", result.RowsAffected)
		}
		var err error
		saved, err = NewStore(tx).FindProvider(ctx, provider.ID)
		return err
	})
	if err != nil {
		return domain.Provider{}, fmt.Errorf("create provider transaction: %w", err)
	}
	return saved, nil
}

type providerCreatedAuditEvent struct {
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
		RequestID string `json:"request_id"`
		After     struct {
			Key              string                `json:"key"`
			AdapterKey       string                `json:"adapter_key"`
			Region           domain.Region         `json:"region"`
			Status           domain.ProviderStatus `json:"status"`
			ConcurrencyLimit int                   `json:"concurrency_limit"`
			RateLimitPerMin  int                   `json:"rate_limit_per_min"`
		} `json:"after"`
	} `json:"data"`
}

func validProviderCreatedAudit(actorID, orgID uuid.UUID, provider domain.Provider, event identityapp.OutboxEvent) bool {
	if event.ID == uuid.Nil || event.Topic != "lanverse.audit.recorded.v1" ||
		event.PartitionKey != orgID.String() || len(event.Payload) == 0 || len(event.Payload) > 32*1024 {
		return false
	}
	var payload providerCreatedAuditEvent
	if !decodeCredentialEvent(event.Payload, &payload) || payload.EventID != event.ID ||
		payload.EventType != event.Topic || payload.OccurredAt.IsZero() || payload.OrgID != orgID ||
		payload.Actor.Kind != "user" || payload.Actor.ID != actorID ||
		payload.Aggregate.Type != "audit" || payload.Aggregate.ID != event.ID ||
		payload.Data.Action != "provider.created" || payload.Data.Object.Type != "provider" ||
		payload.Data.Object.ID != provider.ID || payload.Data.After.Key != provider.Key ||
		payload.Data.After.AdapterKey != provider.AdapterKey || payload.Data.After.Region != provider.Region ||
		payload.Data.After.Status != provider.Status ||
		payload.Data.After.ConcurrencyLimit != provider.ConcurrencyLimit ||
		payload.Data.After.RateLimitPerMin != provider.RateLimitPerMin {
		return false
	}
	requestID, err := uuid.Parse(payload.Data.RequestID)
	return err == nil && requestID != uuid.Nil && requestID.String() == payload.Data.RequestID
}

var _ application.CreateProviderStore = (*Store)(nil)
