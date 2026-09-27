package postgres

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// SetModelStatusWithAudit rechecks current administrator rights and commits
// the model status, revision, and audit in one PostgreSQL transaction.
func (s *Store) SetModelStatusWithAudit(ctx context.Context, actorID, orgID, modelID uuid.UUID, status domain.ModelStatus, expectedRevision int64, event identityapp.OutboxEvent) (domain.ModelProfile, error) {
	if s == nil || s.db == nil || actorID == uuid.Nil || orgID == uuid.Nil || modelID == uuid.Nil ||
		expectedRevision < 1 || expectedRevision >= math.MaxInt32 ||
		!validModelStatusAudit(actorID, orgID, modelID, status, expectedRevision, event) {
		return domain.ModelProfile{}, application.ErrInvalidSetModelStatus
	}
	var saved domain.ModelProfile
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentCatalogAdmin(tx, actorID, orgID); err != nil {
			return err
		}
		if err := setModelStatusInTx(tx, modelID, status, expectedRevision); err != nil {
			return err
		}
		result := tx.Exec(`
			INSERT INTO infra.outbox (id, topic, partition_key, payload)
			VALUES (?::uuid, ?, ?, ?::jsonb)
		`, event.ID.String(), event.Topic, event.PartitionKey, string(event.Payload))
		if result.Error != nil {
			return fmt.Errorf("insert model status audit event: %w", result.Error)
		}
		if err := requireOneRow(result, "insert model status audit event"); err != nil {
			return err
		}
		var row modelProfileRow
		result = tx.Raw(`
			SELECT id, model_key, provider_id, capability, display_name, status,
			       current_version_id, revision, create_time, update_time
			FROM catalog.model_profile WHERE id = ?::uuid AND NOT is_delete
		`, modelID.String()).Scan(&row)
		if result.Error != nil {
			return fmt.Errorf("read changed model status: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrModelNotFound
		}
		saved = row.model()
		if saved.Status != status || saved.Revision != expectedRevision+1 || saved.UpdateTime.IsZero() {
			return domain.ErrModelRevisionConflict
		}
		return nil
	})
	if err != nil {
		return domain.ModelProfile{}, fmt.Errorf("set model status transaction: %w", err)
	}
	return saved, nil
}

type modelStatusAuditSummary struct {
	Status   domain.ModelStatus `json:"status"`
	Revision int64              `json:"revision"`
}

type modelStatusAuditEvent struct {
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
		RequestID string                  `json:"request_id"`
		Before    modelStatusAuditSummary `json:"before"`
		After     modelStatusAuditSummary `json:"after"`
	} `json:"data"`
}

func validModelStatusAudit(actorID, orgID, modelID uuid.UUID, status domain.ModelStatus, expectedRevision int64, event identityapp.OutboxEvent) bool {
	var action string
	var previousStatus domain.ModelStatus
	switch status {
	case domain.ModelActive:
		action, previousStatus = "model.enabled", domain.ModelDisabled
	case domain.ModelDisabled:
		action, previousStatus = "model.disabled", domain.ModelActive
	default:
		return false
	}
	if event.ID == uuid.Nil || event.Topic != "lanverse.audit.recorded.v1" ||
		event.PartitionKey != orgID.String() || len(event.Payload) == 0 || len(event.Payload) > 32*1024 {
		return false
	}
	var payload modelStatusAuditEvent
	if !decodeCredentialEvent(event.Payload, &payload) || payload.EventID != event.ID ||
		payload.EventType != event.Topic || payload.OccurredAt.IsZero() || payload.OrgID != orgID ||
		payload.Actor.Kind != "user" || payload.Actor.ID != actorID ||
		payload.Aggregate.Type != "audit" || payload.Aggregate.ID != event.ID ||
		payload.Data.Action != action || payload.Data.Object.Type != "model_profile" ||
		payload.Data.Object.ID != modelID ||
		payload.Data.Before.Status != previousStatus || payload.Data.Before.Revision != expectedRevision ||
		payload.Data.After.Status != status || payload.Data.After.Revision != expectedRevision+1 {
		return false
	}
	requestID, err := uuid.Parse(payload.Data.RequestID)
	return err == nil && requestID != uuid.Nil && requestID.String() == payload.Data.RequestID
}

var _ application.SetModelStatusStore = (*Store)(nil)
