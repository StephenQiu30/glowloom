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

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// CreateProjectWithEvents commits a new project, its zero budget, the audit,
// and its change event after rechecking the actor and preset in one transaction.
func (s *Store) CreateProjectWithEvents(ctx context.Context, actor identityapp.Principal, project domain.Project, events []identityapp.OutboxEvent) (domain.Project, error) {
	if s == nil || s.db == nil {
		return domain.Project{}, ErrUnavailable
	}
	if err := project.Validate(); err != nil {
		return domain.Project{}, err
	}
	if project.OrgID != actor.OrgID ||
		project.Status != "active" || project.Revision != 1 || project.AllowOverseasModels ||
		project.IsDelete || project.ArchivedAt != nil || project.DeleteTime != nil ||
		project.PurgeAfter != nil || !validProjectCreatedEvents(actor, project, events) {
		return domain.Project{}, application.ErrInvalidCreateProject
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.MustChangePassword ||
		(actor.Role != identitydomain.RoleAdmin && actor.Role != identitydomain.RoleProducer) {
		return domain.Project{}, identityapp.ErrForbidden
	}
	var saved domain.Project
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := requireUsablePreset(tx, project); err != nil {
			return err
		}
		if err := createProjectRowsInTx(tx, project); err != nil {
			return err
		}
		for _, event := range events {
			result := tx.Exec(`
				INSERT INTO infra.outbox (id, topic, partition_key, payload)
				VALUES (?::uuid, ?, ?, ?::jsonb)
			`, event.ID.String(), event.Topic, event.PartitionKey, string(event.Payload))
			if result.Error != nil {
				return fmt.Errorf("insert project event %s: %w", event.Topic, result.Error)
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("insert project event %s: inserted %d rows", event.Topic, result.RowsAffected)
			}
		}
		var err error
		saved, err = readProjectInTx(tx, actor.OrgID, project.ID)
		return err
	})
	if err != nil {
		return domain.Project{}, fmt.Errorf("create project transaction: %w", err)
	}
	return saved, nil
}

type projectEvent struct {
	EventID    uuid.UUID `json:"event_id"`
	EventType  string    `json:"event_type"`
	OccurredAt time.Time `json:"occurred_at"`
	OrgID      uuid.UUID `json:"org_id"`
	ProjectID  uuid.UUID `json:"project_id"`
	Actor      struct {
		Kind string    `json:"kind"`
		ID   uuid.UUID `json:"id"`
	} `json:"actor"`
	Aggregate struct {
		Type     string    `json:"type"`
		ID       uuid.UUID `json:"id"`
		Revision *int64    `json:"revision,omitempty"`
	} `json:"aggregate"`
	Data json.RawMessage `json:"data"`
}

type projectChangedData struct {
	Change string `json:"change"`
}

type projectCreatedAuditData struct {
	Action string `json:"action"`
	Object struct {
		Type string    `json:"type"`
		ID   uuid.UUID `json:"id"`
	} `json:"object"`
	RequestID string `json:"request_id"`
	After     struct {
		AspectRatio   string     `json:"aspect_ratio"`
		StyleType     string     `json:"style_type"`
		StyleSubtype  *string    `json:"style_subtype,omitempty"`
		StylePresetID *uuid.UUID `json:"style_preset_id,omitempty"`
		Status        string     `json:"status"`
		Revision      int64      `json:"revision"`
	} `json:"after"`
}

func validProjectCreatedEvents(actor identityapp.Principal, project domain.Project, events []identityapp.OutboxEvent) bool {
	if len(events) != 2 || events[0].ID == events[1].ID {
		return false
	}
	var changedAt, auditedAt time.Time
	seen := make(map[string]bool, len(events))
	for _, event := range events {
		if event.ID == uuid.Nil || event.PartitionKey != project.ID.String() ||
			len(event.Payload) == 0 || len(event.Payload) > 32*1024 || seen[event.Topic] {
			return false
		}
		seen[event.Topic] = true
		var envelope projectEvent
		if !decodeProjectEvent(event.Payload, &envelope) || envelope.EventID != event.ID ||
			envelope.EventType != event.Topic || envelope.OccurredAt.IsZero() ||
			envelope.OrgID != actor.OrgID || envelope.ProjectID != project.ID ||
			envelope.Actor.Kind != "user" || envelope.Actor.ID != actor.ID {
			return false
		}
		switch event.Topic {
		case "lanverse.workspace.project_changed.v1":
			var data projectChangedData
			if envelope.Aggregate.Type != "project" || envelope.Aggregate.ID != project.ID ||
				envelope.Aggregate.Revision == nil || *envelope.Aggregate.Revision != 1 ||
				!decodeProjectEvent(envelope.Data, &data) || data.Change != "created" {
				return false
			}
			changedAt = envelope.OccurredAt
		case "lanverse.audit.recorded.v1":
			var data projectCreatedAuditData
			if envelope.Aggregate.Type != "audit" || envelope.Aggregate.ID != event.ID ||
				envelope.Aggregate.Revision != nil || !decodeProjectEvent(envelope.Data, &data) ||
				data.Action != "project.created" || data.Object.Type != "project" ||
				data.Object.ID != project.ID || data.After.AspectRatio != project.AspectRatio ||
				data.After.StyleType != project.StyleType || data.After.Status != project.Status ||
				data.After.Revision != project.Revision {
				return false
			}
			if project.StyleSubtype == "" {
				if data.After.StyleSubtype != nil {
					return false
				}
			} else if data.After.StyleSubtype == nil || *data.After.StyleSubtype != project.StyleSubtype {
				return false
			}
			if project.StylePresetID == uuid.Nil {
				if data.After.StylePresetID != nil {
					return false
				}
			} else if data.After.StylePresetID == nil || *data.After.StylePresetID != project.StylePresetID {
				return false
			}
			requestID, err := uuid.Parse(data.RequestID)
			if err != nil || requestID == uuid.Nil || requestID.String() != data.RequestID {
				return false
			}
			if _, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
				Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Payload,
			}); err != nil {
				return false
			}
			auditedAt = envelope.OccurredAt
		default:
			return false
		}
	}
	return seen["lanverse.workspace.project_changed.v1"] &&
		seen["lanverse.audit.recorded.v1"] && changedAt.Equal(auditedAt)
}

func decodeProjectEvent[T any](payload json.RawMessage, target *T) bool {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return false
	}
	return decoder.Decode(new(any)) == io.EOF
}

var _ application.CreateProjectStore = (*Store)(nil)
