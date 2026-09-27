package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	"github.com/google/uuid"
	"gorm.io/gorm"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// UpdateProjectWithEvents locks the live project and commits settings and both
// durable events only after rechecking current authorization and preset scope.
func (s *Store) UpdateProjectWithEvents(ctx context.Context, actor identityapp.Principal, before, after domain.Project, events []identityapp.OutboxEvent) (domain.Project, error) {
	if s == nil || s.db == nil {
		return domain.Project{}, ErrUnavailable
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.MustChangePassword ||
		(actor.Role != identitydomain.RoleAdmin && actor.Role != identitydomain.RoleProducer) {
		return domain.Project{}, identityapp.ErrForbidden
	}
	if before.Validate() != nil || after.Validate() != nil ||
		before.ID != after.ID || before.OrgID != actor.OrgID || after.OrgID != actor.OrgID ||
		before.AspectRatio != after.AspectRatio || before.StyleType != after.StyleType ||
		before.StyleSubtype != after.StyleSubtype || before.Resolution != after.Resolution ||
		before.Status != "active" || after.Status != "active" || before.IsDelete || after.IsDelete ||
		before.ArchivedAt != nil || after.ArchivedAt != nil ||
		before.DeleteTime != nil || after.DeleteTime != nil ||
		before.PurgeAfter != nil || after.PurgeAfter != nil ||
		!before.CreateTime.Equal(after.CreateTime) || !before.UpdateTime.Equal(after.UpdateTime) ||
		before.Revision+1 != after.Revision || sameStoredProjectSettings(before, after) ||
		!validProjectUpdatedEvents(actor, before, after, events) {
		return domain.Project{}, application.ErrInvalidUpdateProject
	}
	var saved domain.Project
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		var locked int
		result := tx.Raw(`
			SELECT 1 FROM workspace.project
			WHERE id = ?::uuid AND org_id = ?::uuid FOR UPDATE
		`, before.ID.String(), actor.OrgID.String()).Scan(&locked)
		if result.Error != nil {
			return fmt.Errorf("lock project for update: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrProjectNotFound
		}
		current, err := readProjectInTx(tx, actor.OrgID, before.ID)
		if err != nil {
			return err
		}
		if err := current.CanWrite(); err != nil {
			return err
		}
		if current.Revision != before.Revision || !sameStoredProjectSettings(current, before) ||
			current.StyleType != before.StyleType || current.StyleSubtype != before.StyleSubtype ||
			current.AspectRatio != before.AspectRatio || current.Resolution != before.Resolution {
			return domain.ErrProjectRevisionConflict
		}
		if err := requireUsablePreset(tx, after); err != nil {
			return err
		}
		var presetID any
		if after.StylePresetID != uuid.Nil {
			presetID = after.StylePresetID.String()
		}
		result = tx.Exec(`
			UPDATE workspace.project
			SET name = ?, description = ?, style_preset_id = ?::uuid,
			    allow_overseas_models = ?, revision = revision + 1
			WHERE id = ?::uuid AND org_id = ?::uuid AND revision = ?
			  AND status = 'active' AND NOT is_delete
		`, after.Name, after.Description, presetID, after.AllowOverseasModels,
			after.ID.String(), actor.OrgID.String(), before.Revision)
		if result.Error != nil {
			return fmt.Errorf("update project settings: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return domain.ErrProjectRevisionConflict
		}
		for _, event := range events {
			result := tx.Exec(`
				INSERT INTO infra.outbox (id, topic, partition_key, payload)
				VALUES (?::uuid, ?, ?, ?::jsonb)
			`, event.ID.String(), event.Topic, event.PartitionKey, string(event.Payload))
			if result.Error != nil {
				return fmt.Errorf("insert project update event %s: %w", event.Topic, result.Error)
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("insert project update event %s: inserted %d rows", event.Topic, result.RowsAffected)
			}
		}
		saved, err = readProjectInTx(tx, actor.OrgID, after.ID)
		return err
	})
	if err != nil {
		return domain.Project{}, fmt.Errorf("update project transaction: %w", err)
	}
	return saved, nil
}

func sameStoredProjectSettings(left, right domain.Project) bool {
	return left.Name == right.Name && left.Description == right.Description &&
		left.StylePresetID == right.StylePresetID &&
		left.AllowOverseasModels == right.AllowOverseasModels
}

type projectUpdatedAuditData struct {
	Action string `json:"action"`
	Object struct {
		Type string    `json:"type"`
		ID   uuid.UUID `json:"id"`
	} `json:"object"`
	RequestID string          `json:"request_id"`
	Before    json.RawMessage `json:"before"`
	After     json.RawMessage `json:"after"`
}

func validProjectUpdatedEvents(actor identityapp.Principal, before, after domain.Project, events []identityapp.OutboxEvent) bool {
	if len(events) != 2 || events[0].ID == uuid.Nil || events[1].ID == uuid.Nil ||
		events[0].ID == events[1].ID || events[0].Topic != "lanverse.workspace.project_changed.v1" ||
		events[1].Topic != "lanverse.audit.recorded.v1" {
		return false
	}
	for _, event := range events {
		if event.PartitionKey != after.ID.String() || len(event.Payload) == 0 || len(event.Payload) > 32*1024 {
			return false
		}
	}
	var changed projectEvent
	var changeData projectChangedData
	if !decodeProjectEvent(events[0].Payload, &changed) ||
		changed.EventID != events[0].ID || changed.EventType != events[0].Topic ||
		changed.OccurredAt.IsZero() || changed.OrgID != actor.OrgID ||
		changed.ProjectID != after.ID || changed.Actor.Kind != "user" || changed.Actor.ID != actor.ID ||
		changed.Aggregate.Type != "project" || changed.Aggregate.ID != after.ID ||
		changed.Aggregate.Revision == nil || *changed.Aggregate.Revision != after.Revision ||
		!decodeProjectEvent(changed.Data, &changeData) || changeData.Change != "updated" {
		return false
	}
	var auditEvent projectEvent
	var auditData projectUpdatedAuditData
	if !decodeProjectEvent(events[1].Payload, &auditEvent) ||
		auditEvent.EventID != events[1].ID || auditEvent.EventType != events[1].Topic ||
		!auditEvent.OccurredAt.Equal(changed.OccurredAt) || auditEvent.OrgID != actor.OrgID ||
		auditEvent.ProjectID != after.ID || auditEvent.Actor.Kind != "user" || auditEvent.Actor.ID != actor.ID ||
		auditEvent.Aggregate.Type != "audit" || auditEvent.Aggregate.ID != events[1].ID ||
		auditEvent.Aggregate.Revision != nil || !decodeProjectEvent(auditEvent.Data, &auditData) ||
		auditData.Action != "project.updated" || auditData.Object.Type != "project" ||
		auditData.Object.ID != after.ID || !validProjectRequestID(auditData.RequestID) {
		return false
	}
	if _, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
		Topic: events[1].Topic, Key: []byte(events[1].PartitionKey), Value: events[1].Payload,
	}); err != nil {
		return false
	}
	wantBefore, wantAfter := expectedProjectUpdateSummaries(before, after)
	return sameJSONSummary(auditData.Before, wantBefore) && sameJSONSummary(auditData.After, wantAfter)
}

func validProjectRequestID(raw string) bool {
	id, err := uuid.Parse(raw)
	return err == nil && id != uuid.Nil && id.String() == raw
}

func expectedProjectUpdateSummaries(before, after domain.Project) (map[string]any, map[string]any) {
	old := map[string]any{"revision": before.Revision}
	afterSummary := map[string]any{"revision": after.Revision}
	if before.StylePresetID != after.StylePresetID {
		var oldID, newID any
		if before.StylePresetID != uuid.Nil {
			oldID = before.StylePresetID.String()
		}
		if after.StylePresetID != uuid.Nil {
			newID = after.StylePresetID.String()
		}
		old["style_preset_id"], afterSummary["style_preset_id"] = oldID, newID
	}
	if before.AllowOverseasModels != after.AllowOverseasModels {
		old["allow_overseas_models"] = before.AllowOverseasModels
		afterSummary["allow_overseas_models"] = after.AllowOverseasModels
	}
	if before.Name != after.Name {
		afterSummary["name_changed"] = true
	}
	if before.Description != after.Description {
		afterSummary["description_changed"] = true
	}
	return old, afterSummary
}

func sameJSONSummary(raw json.RawMessage, expected map[string]any) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var actual map[string]any
	if err := decoder.Decode(&actual); err != nil || decoder.Decode(new(any)) != io.EOF {
		return false
	}
	encoded, err := json.Marshal(expected)
	if err != nil {
		return false
	}
	var want map[string]any
	return json.Unmarshal(encoded, &want) == nil && reflect.DeepEqual(actual, want)
}

var _ application.UpdateProjectStore = (*Store)(nil)
