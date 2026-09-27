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

// PublishModelVersionWithAudit rechecks current rights and commits the
// immutable version, current pointer, and safe audit in one transaction.
func (s *Store) PublishModelVersionWithAudit(ctx context.Context, actorID, orgID uuid.UUID, version domain.ModelVersion, expectedRevision int64, event identityapp.OutboxEvent) (domain.ModelVersion, error) {
	if s == nil || s.db == nil || actorID == uuid.Nil || orgID == uuid.Nil ||
		version.Validate() != nil || version.CreateBy != actorID || expectedRevision < 1 ||
		!validModelVersionPublishedAudit(actorID, orgID, version, expectedRevision, event) {
		return domain.ModelVersion{}, application.ErrInvalidPublishModelVersion
	}
	var saved domain.ModelVersion
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentCatalogAdmin(tx, actorID, orgID); err != nil {
			return err
		}
		if err := appendModelVersionInTx(tx, version, expectedRevision); err != nil {
			return err
		}
		result := tx.Exec(`
			INSERT INTO infra.outbox (id, topic, partition_key, payload)
			VALUES (?::uuid, ?, ?, ?::jsonb)
		`, event.ID.String(), event.Topic, event.PartitionKey, string(event.Payload))
		if result.Error != nil {
			return fmt.Errorf("insert model version audit event: %w", result.Error)
		}
		if err := requireOneRow(result, "insert model version audit event"); err != nil {
			return err
		}
		var row struct {
			CreateTime time.Time
		}
		result = tx.Raw(`
			SELECT create_time FROM catalog.model_profile_version
			WHERE id = ?::uuid AND model_profile_id = ?::uuid AND NOT is_delete
		`, version.ID.String(), version.ModelID.String()).Scan(&row)
		if result.Error != nil {
			return fmt.Errorf("read published model version: %w", result.Error)
		}
		if result.RowsAffected != 1 || row.CreateTime.IsZero() {
			return ErrModelVersionConflict
		}
		saved = version
		saved.CreateTime = row.CreateTime
		return nil
	})
	if err != nil {
		return domain.ModelVersion{}, fmt.Errorf("publish model version transaction: %w", err)
	}
	return saved, nil
}

type modelVersionPublishedAuditEvent struct {
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
			VersionID       uuid.UUID `json:"version_id"`
			VersionNo       int       `json:"version_no"`
			ProviderModelID string    `json:"provider_model_id"`
			Revision        int64     `json:"revision"`
		} `json:"after"`
	} `json:"data"`
}

func validModelVersionPublishedAudit(actorID, orgID uuid.UUID, version domain.ModelVersion, expectedRevision int64, event identityapp.OutboxEvent) bool {
	if event.ID == uuid.Nil || event.Topic != "lanverse.audit.recorded.v1" ||
		event.PartitionKey != orgID.String() || len(event.Payload) == 0 || len(event.Payload) > 32*1024 {
		return false
	}
	var payload modelVersionPublishedAuditEvent
	if !decodeCredentialEvent(event.Payload, &payload) || payload.EventID != event.ID ||
		payload.EventType != event.Topic || payload.OccurredAt.IsZero() || payload.OrgID != orgID ||
		payload.Actor.Kind != "user" || payload.Actor.ID != actorID ||
		payload.Aggregate.Type != "audit" || payload.Aggregate.ID != event.ID ||
		payload.Data.Action != "model.version_published" ||
		payload.Data.Object.Type != "model_profile" || payload.Data.Object.ID != version.ModelID ||
		payload.Data.After.VersionID != version.ID || payload.Data.After.VersionNo != version.VersionNo ||
		payload.Data.After.ProviderModelID != version.ProviderModelID ||
		payload.Data.After.Revision != expectedRevision+1 {
		return false
	}
	requestID, err := uuid.Parse(payload.Data.RequestID)
	return err == nil && requestID != uuid.Nil && requestID.String() == payload.Data.RequestID
}

var _ application.PublishModelVersionStore = (*Store)(nil)
