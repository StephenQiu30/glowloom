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

// CreateModelWithAudit rechecks the administrator and commits the new model
// identity and audit event in one PostgreSQL transaction.
func (s *Store) CreateModelWithAudit(ctx context.Context, actorID, orgID uuid.UUID, model domain.ModelProfile, event identityapp.OutboxEvent) (domain.ModelProfile, error) {
	if s == nil || s.db == nil || actorID == uuid.Nil || orgID == uuid.Nil ||
		model.Validate() != nil || model.Status != domain.ModelDisabled ||
		model.CurrentVersionID != uuid.Nil || model.Revision != 1 ||
		!validModelCreatedAudit(actorID, orgID, model, event) {
		return domain.ModelProfile{}, application.ErrInvalidCreateModel
	}
	var saved domain.ModelProfile
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentCatalogAdmin(tx, actorID, orgID); err != nil {
			return err
		}
		if err := createModelInTx(tx, model); err != nil {
			return err
		}
		result := tx.Exec(`
			INSERT INTO infra.outbox (id, topic, partition_key, payload)
			VALUES (?::uuid, ?, ?, ?::jsonb)
		`, event.ID.String(), event.Topic, event.PartitionKey, string(event.Payload))
		if result.Error != nil {
			return fmt.Errorf("insert model audit event: %w", result.Error)
		}
		if err := requireOneRow(result, "insert model audit event"); err != nil {
			return err
		}
		var row modelProfileRow
		result = tx.Raw(`
			SELECT id, model_key, provider_id, capability, display_name, status,
			       current_version_id, revision, create_time, update_time
			FROM catalog.model_profile WHERE id = ?::uuid AND NOT is_delete
		`, model.ID.String()).Scan(&row)
		if result.Error != nil {
			return fmt.Errorf("read created model: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrModelNotFound
		}
		saved = row.model()
		return nil
	})
	if err != nil {
		return domain.ModelProfile{}, fmt.Errorf("create model transaction: %w", err)
	}
	return saved, nil
}

type modelCreatedAuditEvent struct {
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
			Key         string             `json:"key"`
			ProviderID  uuid.UUID          `json:"provider_id"`
			Capability  string             `json:"capability"`
			DisplayName string             `json:"display_name"`
			Status      domain.ModelStatus `json:"status"`
		} `json:"after"`
	} `json:"data"`
}

func validModelCreatedAudit(actorID, orgID uuid.UUID, model domain.ModelProfile, event identityapp.OutboxEvent) bool {
	if event.ID == uuid.Nil || event.Topic != "lanverse.audit.recorded.v1" ||
		event.PartitionKey != orgID.String() || len(event.Payload) == 0 || len(event.Payload) > 32*1024 {
		return false
	}
	var payload modelCreatedAuditEvent
	if !decodeCredentialEvent(event.Payload, &payload) || payload.EventID != event.ID ||
		payload.EventType != event.Topic || payload.OccurredAt.IsZero() || payload.OrgID != orgID ||
		payload.Actor.Kind != "user" || payload.Actor.ID != actorID ||
		payload.Aggregate.Type != "audit" || payload.Aggregate.ID != event.ID ||
		payload.Data.Action != "model.created" || payload.Data.Object.Type != "model_profile" ||
		payload.Data.Object.ID != model.ID || payload.Data.After.Key != model.Key ||
		payload.Data.After.ProviderID != model.ProviderID ||
		payload.Data.After.Capability != model.Capability ||
		payload.Data.After.DisplayName != model.DisplayName ||
		payload.Data.After.Status != model.Status {
		return false
	}
	requestID, err := uuid.Parse(payload.Data.RequestID)
	return err == nil && requestID != uuid.Nil && requestID.String() == payload.Data.RequestID
}

var _ application.CreateModelStore = (*Store)(nil)
