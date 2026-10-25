package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// UpdateProviderWithEvents locks the current provider and writes the new
// settings, cache event, and audit event in one transaction.
func (s *Store) UpdateProviderWithEvents(ctx context.Context, actor identityapp.Principal, before, after domain.Provider, events []identityapp.OutboxEvent, input application.UpdateProviderInput) (domain.Provider, error) {
	key, err := uuid.Parse(input.RequestID)
	if err != nil || key == uuid.Nil || input.ProviderID != before.ID || input.ExpectedRevision != before.Revision ||
		(input.Status != nil && *input.Status != after.Status) ||
		(input.ConcurrencyLimit != nil && *input.ConcurrencyLimit != after.ConcurrencyLimit) ||
		(input.RateLimitPerMin != nil && *input.RateLimitPerMin != after.RateLimitPerMin) {
		return domain.Provider{}, application.ErrInvalidUpdateProvider
	}
	return catalogRequest(ctx, s.db, s.actors, actor, "provider.update", input.ProviderID, key, input, false, func(tx *gorm.DB) (domain.Provider, error) {
		return s.withDB(tx).updateProviderWithEvents(ctx, actor, before, after, events)
	})
}

// FindProviderUpdateReceipt rechecks authority and returns only an exact original request.
func (s *Store) FindProviderUpdateReceipt(ctx context.Context, actor identityapp.Principal, input application.UpdateProviderInput) (application.CreatedProvider, bool, error) {
	var out application.CreatedProvider
	found := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requirePlatformAdmin(tx, actor); err != nil {
			return err
		}
		lock := actor.ID.String() + "/provider.update/" + input.ProviderID.String() + "/" + input.RequestID
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, lock).Error; err != nil {
			return err
		}
		var row struct {
			Fingerprint string
			Result      []byte
		}
		r := tx.Raw(`SELECT fingerprint,result FROM catalog.config_request WHERE actor_id=? AND action='provider.update' AND target_id=? AND request_id=?`, actor.ID, input.ProviderID, input.RequestID).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return nil
		}
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(data)
		if row.Fingerprint != hex.EncodeToString(digest[:]) {
			return application.ErrPresetConflict
		}
		var model domain.Provider
		if err := json.Unmarshal(row.Result, &model); err != nil {
			return err
		}
		out = application.CreatedProvider{ID: model.ID, Key: model.Key, Name: model.Name, AdapterKey: model.AdapterKey, Region: model.Region, Status: model.Status, ConcurrencyLimit: model.ConcurrencyLimit, RateLimitPerMin: model.RateLimitPerMin, Revision: model.Revision, CreateTime: model.CreateTime, UpdateTime: model.UpdateTime}
		found = true
		return nil
	})
	return out, found, err
}

func (s *Store) updateProviderWithEvents(ctx context.Context, actor identityapp.Principal, before, after domain.Provider, events []identityapp.OutboxEvent) (domain.Provider, error) {
	if s == nil || s.db == nil || actor.ID == uuid.Nil || actor.OrgID == uuid.Nil ||
		before.Validate() != nil || after.Validate() != nil ||
		before.ID != after.ID || before.Key != after.Key || before.Name != after.Name ||
		before.AdapterKey != after.AdapterKey || before.Region != after.Region ||
		before.Revision+1 != after.Revision ||
		(before.Status == after.Status && before.ConcurrencyLimit == after.ConcurrencyLimit &&
			before.RateLimitPerMin == after.RateLimitPerMin) ||
		!validProviderUpdateEvents(actor.ID, actor.OrgID, before, after, events) {
		return domain.Provider{}, application.ErrInvalidUpdateProvider
	}
	var saved domain.Provider
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requirePlatformAdmin(tx, actor); err != nil {
			return err
		}
		var row providerRow
		result := tx.Raw(`
			SELECT id, key, name, adapter_key, region, status, concurrency_limit,
			       rate_limit_per_min, revision, create_time, update_time
			FROM catalog.provider WHERE id = ?::uuid AND NOT is_delete FOR UPDATE
		`, before.ID.String()).Scan(&row)
		if result.Error != nil {
			return fmt.Errorf("lock provider for update: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrProviderNotFound
		}
		current := row.provider()
		if current.Revision != before.Revision || current.Key != before.Key ||
			current.Name != before.Name || current.AdapterKey != before.AdapterKey ||
			current.Region != before.Region || current.Status != before.Status ||
			current.ConcurrencyLimit != before.ConcurrencyLimit ||
			current.RateLimitPerMin != before.RateLimitPerMin {
			return ErrRevisionConflict
		}
		if err := NewStore(tx).UpdateProvider(ctx, after, before.Revision); err != nil {
			return err
		}
		for _, event := range events {
			result := tx.Exec(`
				INSERT INTO infra.outbox (id, topic, partition_key, payload)
				VALUES (?::uuid, ?, ?, ?::jsonb)
			`, event.ID.String(), event.Topic, event.PartitionKey, string(event.Payload))
			if result.Error != nil {
				return fmt.Errorf("insert provider update event %s: %w", event.Topic, result.Error)
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("insert provider update event %s: inserted %d rows", event.Topic, result.RowsAffected)
			}
		}
		var err error
		saved, err = NewStore(tx).FindProvider(ctx, after.ID)
		return err
	})
	if err != nil {
		return domain.Provider{}, fmt.Errorf("update provider transaction: %w", err)
	}
	return saved, nil
}

type providerUpdateChangedEvent struct {
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
		ProviderID uuid.UUID `json:"provider_id"`
		Revision   int64     `json:"revision"`
	} `json:"data"`
}

type providerUpdateAuditEvent struct {
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
		Before    struct {
			Status           domain.ProviderStatus `json:"status"`
			ConcurrencyLimit int                   `json:"concurrency_limit"`
			RateLimitPerMin  int                   `json:"rate_limit_per_min"`
		} `json:"before"`
		After struct {
			Status           domain.ProviderStatus `json:"status"`
			ConcurrencyLimit int                   `json:"concurrency_limit"`
			RateLimitPerMin  int                   `json:"rate_limit_per_min"`
		} `json:"after"`
	} `json:"data"`
}

func validProviderUpdateEvents(actorID, orgID uuid.UUID, before, after domain.Provider, events []identityapp.OutboxEvent) bool {
	if len(events) != 2 || events[0].ID == uuid.Nil || events[1].ID == uuid.Nil || events[0].ID == events[1].ID ||
		events[0].Topic != "glowloom.catalog.provider_changed.v1" || events[1].Topic != "glowloom.audit.recorded.v1" {
		return false
	}
	for _, event := range events {
		if event.PartitionKey != orgID.String() || len(event.Payload) == 0 || len(event.Payload) > 32*1024 {
			return false
		}
	}
	var changed providerUpdateChangedEvent
	if !decodeCredentialEvent(events[0].Payload, &changed) || changed.EventID != events[0].ID ||
		changed.EventType != events[0].Topic || changed.OccurredAt.IsZero() || changed.OrgID != orgID ||
		changed.Actor.Kind != "user" || changed.Actor.ID != actorID ||
		changed.Aggregate.Type != "provider" || changed.Aggregate.ID != after.ID ||
		changed.Data.ProviderID != after.ID || changed.Data.Revision != after.Revision {
		return false
	}
	var audit providerUpdateAuditEvent
	if !decodeCredentialEvent(events[1].Payload, &audit) || audit.EventID != events[1].ID ||
		audit.EventType != events[1].Topic || !audit.OccurredAt.Equal(changed.OccurredAt) ||
		audit.OrgID != orgID || audit.Actor.Kind != "user" || audit.Actor.ID != actorID ||
		audit.Aggregate.Type != "audit" || audit.Aggregate.ID != audit.EventID ||
		audit.Data.Action != "provider.updated" || audit.Data.Object.Type != "provider" ||
		audit.Data.Object.ID != after.ID ||
		audit.Data.Before.Status != before.Status ||
		audit.Data.Before.ConcurrencyLimit != before.ConcurrencyLimit ||
		audit.Data.Before.RateLimitPerMin != before.RateLimitPerMin ||
		audit.Data.After.Status != after.Status ||
		audit.Data.After.ConcurrencyLimit != after.ConcurrencyLimit ||
		audit.Data.After.RateLimitPerMin != after.RateLimitPerMin {
		return false
	}
	requestID, err := uuid.Parse(audit.Data.RequestID)
	return err == nil && requestID != uuid.Nil && requestID.String() == audit.Data.RequestID
}

var _ application.UpdateProviderStore = (*Store)(nil)
