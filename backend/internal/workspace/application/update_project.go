package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// ErrInvalidUpdateProject means the patch or committed result is invalid.
var ErrInvalidUpdateProject = errors.New("invalid update-project command")

// UpdatedProject reuses the safe project response without duplicating its fields.
type UpdatedProject = CreatedProject

// UpdateProjectStore reads the current project and commits its guarded update.
type UpdateProjectStore interface {
	FindProject(context.Context, identityapp.Principal, uuid.UUID) (domain.Project, error)
	UpdateProjectWithEvents(context.Context, identityapp.Principal, domain.Project, domain.Project, []identityapp.OutboxEvent) (domain.Project, error)
}

// UpdateProjectInput contains only mutable settings. A nil pointer leaves a
// field unchanged; StylePresetID pointing to uuid.Nil clears the preset.
type UpdateProjectInput struct {
	ProjectID           uuid.UUID
	ExpectedRevision    int64
	Name                *string
	Description         *string
	StylePresetID       *uuid.UUID
	AllowOverseasModels *bool
	RequestID           string
}

// UpdateProjectCommand changes settings with a revision and durable audit.
type UpdateProjectCommand struct {
	store UpdateProjectStore
	now   func() time.Time
}

// NewUpdateProjectCommand injects persistence and a clock.
func NewUpdateProjectCommand(store UpdateProjectStore, now func() time.Time) *UpdateProjectCommand {
	return &UpdateProjectCommand{store: store, now: now}
}

// Execute applies a valid patch to an active project or returns it unchanged.
func (c *UpdateProjectCommand) Execute(ctx context.Context, actor identityapp.Principal, input UpdateProjectInput) (UpdatedProject, error) {
	if c == nil || c.store == nil || c.now == nil {
		return UpdatedProject{}, ErrInvalidUpdateProject
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.MustChangePassword ||
		(actor.Role != identitydomain.RoleAdmin && actor.Role != identitydomain.RoleProducer) {
		return UpdatedProject{}, identityapp.ErrForbidden
	}
	requestID, err := uuid.Parse(input.RequestID)
	if input.ProjectID == uuid.Nil || input.ExpectedRevision < 1 || input.ExpectedRevision >= math.MaxInt32 ||
		err != nil || requestID == uuid.Nil || requestID.String() != input.RequestID ||
		(input.Name == nil && input.Description == nil && input.StylePresetID == nil && input.AllowOverseasModels == nil) {
		return UpdatedProject{}, ErrInvalidUpdateProject
	}
	if !validProjectPatch(input) {
		return UpdatedProject{}, ErrInvalidUpdateProject
	}
	before, err := c.store.FindProject(ctx, actor, input.ProjectID)
	if err != nil {
		return UpdatedProject{}, fmt.Errorf("find project to update: %w", err)
	}
	if before.ID != input.ProjectID || before.OrgID != actor.OrgID || before.Validate() != nil {
		return UpdatedProject{}, ErrInvalidUpdateProject
	}
	if err := before.CanWrite(); err != nil {
		return UpdatedProject{}, err
	}
	if before.Revision != input.ExpectedRevision {
		return UpdatedProject{}, domain.ErrProjectRevisionConflict
	}
	after := applyProjectPatch(before, input)
	if sameProjectSettings(before, after) {
		return projectSettingsResult(before), nil
	}
	after.Revision++
	if err := after.Validate(); err != nil {
		return UpdatedProject{}, fmt.Errorf("validate updated project: %w", err)
	}
	occurredAt := c.now().UTC()
	if occurredAt.IsZero() {
		return UpdatedProject{}, ErrInvalidUpdateProject
	}
	events, err := projectUpdatedEvents(actor, before, after, input.RequestID, occurredAt)
	if err != nil {
		return UpdatedProject{}, fmt.Errorf("build project update events: %w", err)
	}
	saved, err := c.store.UpdateProjectWithEvents(ctx, actor, before, after, events)
	if err != nil {
		return UpdatedProject{}, fmt.Errorf("update project with events: %w", err)
	}
	if saved.ID != after.ID || saved.OrgID != after.OrgID || !sameProjectSettings(saved, after) ||
		saved.AspectRatio != after.AspectRatio || saved.StyleType != after.StyleType ||
		saved.StyleSubtype != after.StyleSubtype || saved.Resolution != after.Resolution ||
		saved.Status != after.Status || saved.IsDelete != after.IsDelete ||
		saved.Revision != after.Revision || saved.CreateTime.IsZero() || saved.UpdateTime.IsZero() {
		return UpdatedProject{}, ErrInvalidUpdateProject
	}
	return projectSettingsResult(saved), nil
}

func sameProjectSettings(left, right domain.Project) bool {
	return left.Name == right.Name && left.Description == right.Description &&
		left.StylePresetID == right.StylePresetID &&
		left.AllowOverseasModels == right.AllowOverseasModels
}

func projectSettingsResult(project domain.Project) UpdatedProject {
	var presetID *uuid.UUID
	if project.StylePresetID != uuid.Nil {
		id := project.StylePresetID
		presetID = &id
	}
	return UpdatedProject{
		ID: project.ID, OrgID: project.OrgID, Name: project.Name,
		Description: project.Description, AspectRatio: project.AspectRatio,
		StyleType: project.StyleType, StyleSubtype: project.StyleSubtype,
		StylePresetID: presetID, Resolution: project.Resolution,
		AllowOverseasModels: project.AllowOverseasModels, Status: project.Status,
		Revision: project.Revision, CreateTime: project.CreateTime, UpdateTime: project.UpdateTime,
	}
}

func projectUpdateSummaries(before, after domain.Project) (map[string]any, map[string]any) {
	old := map[string]any{"revision": before.Revision}
	afterSummary := map[string]any{"revision": after.Revision}
	if before.StylePresetID != after.StylePresetID {
		var oldID, newID any
		if before.StylePresetID != uuid.Nil {
			oldID = before.StylePresetID
		}
		if after.StylePresetID != uuid.Nil {
			newID = after.StylePresetID
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

func projectUpdatedEvents(actor identityapp.Principal, before, after domain.Project, requestID string, occurredAt time.Time) ([]identityapp.OutboxEvent, error) {
	changeID, auditID := uuid.New(), uuid.New()
	change, err := json.Marshal(map[string]any{
		"event_id": changeID, "event_type": projectChangedTopic,
		"occurred_at": occurredAt, "org_id": actor.OrgID, "project_id": after.ID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "project", "id": after.ID, "revision": after.Revision},
		"data":      map[string]any{"change": "updated"},
	})
	if err != nil {
		return nil, fmt.Errorf("encode project changed event: %w", err)
	}
	oldSummary, newSummary := projectUpdateSummaries(before, after)
	audit, err := json.Marshal(map[string]any{
		"event_id": auditID, "event_type": projectAuditTopic,
		"occurred_at": occurredAt, "org_id": actor.OrgID, "project_id": after.ID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "audit", "id": auditID},
		"data": map[string]any{
			"action": "project.updated", "object": map[string]any{"type": "project", "id": after.ID},
			"request_id": requestID, "before": oldSummary, "after": newSummary,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode project updated audit: %w", err)
	}
	key := after.ID.String()
	return []identityapp.OutboxEvent{
		{ID: changeID, Topic: projectChangedTopic, PartitionKey: key, Payload: change},
		{ID: auditID, Topic: projectAuditTopic, PartitionKey: key, Payload: audit},
	}, nil
}
