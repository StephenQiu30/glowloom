// Package application coordinates project commands across domain and persistence.
package application

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

// CreateProjectStore rechecks current rights and commits the project, its zero
// budget, and both durable events in one transaction.
type CreateProjectStore interface {
	CreateProjectWithEvents(context.Context, identityapp.Principal, domain.Project, []identityapp.OutboxEvent) (domain.Project, error)
	CreateProjectIdempotently(context.Context, identityapp.Principal, domain.Project, []identityapp.OutboxEvent, workspacesource.ProjectCreationRequest) (domain.Project, error)
}

// CreateProjectCommand registers a project with its zero budget and events.
type CreateProjectCommand struct {
	store CreateProjectStore
	now   func() time.Time
}

// NewCreateProjectCommand injects transactional persistence and a clock.
func NewCreateProjectCommand(store CreateProjectStore, now func() time.Time) *CreateProjectCommand {
	return &CreateProjectCommand{store: store, now: now}
}

// Execute validates the actor and immutable specifications before writing.
func (c *CreateProjectCommand) Execute(ctx context.Context, actor identityapp.Principal, input workspacesource.CreateProjectInput) (workspacesource.CreatedProject, error) {
	return c.execute(ctx, actor, input, nil)
}

// ExecuteIdempotent is the public creation entry point. The request identifier
// belongs to the audit; it does not change the logical request fingerprint.
func (c *CreateProjectCommand) ExecuteIdempotent(ctx context.Context, actor identityapp.Principal, key uuid.UUID, input workspacesource.CreateProjectInput) (workspacesource.CreatedProject, error) {
	if key == uuid.Nil {
		return workspacesource.CreatedProject{}, workspacesource.ErrInvalidCreateProject
	}
	hash, err := workspacesource.ProjectCreationSHA256(actor.OrgID, actor.ScopeID, input)
	if err != nil {
		return workspacesource.CreatedProject{}, err
	}
	return c.execute(ctx, actor, input, &workspacesource.ProjectCreationRequest{Key: key, Hash: hash})
}

func (c *CreateProjectCommand) execute(ctx context.Context, actor identityapp.Principal, input workspacesource.CreateProjectInput, request *workspacesource.ProjectCreationRequest) (workspacesource.CreatedProject, error) {
	if c == nil || c.store == nil || c.now == nil {
		return workspacesource.CreatedProject{}, workspacesource.ErrInvalidCreateProject
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil ||
		(actor.ScopeID == uuid.Nil || actor.SessionID == uuid.Nil || actor.CredentialEpoch < 1) {
		return workspacesource.CreatedProject{}, identityapp.ErrForbidden
	}
	requestID, err := uuid.Parse(input.RequestID)
	name := strings.TrimSpace(input.Name)
	if err != nil || requestID == uuid.Nil || requestID.String() != input.RequestID ||
		!utf8.ValidString(name) || !utf8.ValidString(input.Description) {
		return workspacesource.CreatedProject{}, workspacesource.ErrInvalidCreateProject
	}
	if input.StyleType == "" {
		input.StyleType = "realistic"
	}
	project := domain.Project{
		IndependentCanvas: input.IndependentCanvas,
		ID:                uuid.New(), OrgID: actor.OrgID, OwnerID: actor.ID, CanvasID: uuid.New(), StyleKey: input.StyleKey, Name: name, Description: input.Description,
		AspectRatio: input.AspectRatio, StyleType: input.StyleType,
		StyleSubtype: input.StyleSubtype, StylePresetID: input.StylePresetID,
		Resolution: "1080p", Status: "active", Revision: 1,
	}
	if err := project.Validate(); err != nil {
		return workspacesource.CreatedProject{}, workspacesource.ErrInvalidCreateProject
	}
	occurredAt := c.now().UTC()
	if occurredAt.IsZero() {
		return workspacesource.CreatedProject{}, workspacesource.ErrInvalidCreateProject
	}
	events, err := projectCreatedEvents(actor, project, input.RequestID, occurredAt)
	if err != nil {
		return workspacesource.CreatedProject{}, fmt.Errorf("build project events: %w", err)
	}
	var saved domain.Project
	if request == nil {
		saved, err = c.store.CreateProjectWithEvents(ctx, actor, project, events)
	} else {
		saved, err = c.store.CreateProjectIdempotently(ctx, actor, project, events, *request)
	}
	if err != nil {
		return workspacesource.CreatedProject{}, fmt.Errorf("create project with events: %w", err)
	}
	if saved.IndependentCanvas != project.IndependentCanvas || saved.OwnerID != actor.ID || saved.CanvasID == uuid.Nil || (request == nil && saved.CanvasID != project.CanvasID) || !equalStyleKey(saved.StyleKey, project.StyleKey) || saved.ID == uuid.Nil || (request == nil && saved.ID != project.ID) || saved.OrgID != project.OrgID || saved.Name != project.Name ||
		saved.Description != project.Description || saved.AspectRatio != project.AspectRatio ||
		saved.StyleType != project.StyleType || saved.StyleSubtype != project.StyleSubtype ||
		saved.StylePresetID != project.StylePresetID || saved.Resolution != "1080p" ||
		saved.AllowOverseasModels || saved.Status != "active" || saved.Revision != 1 ||
		saved.IsDelete || saved.ArchivedAt != nil || saved.DeleteTime != nil ||
		saved.PurgeAfter != nil || saved.CreateTime.IsZero() || saved.UpdateTime.IsZero() {
		return workspacesource.CreatedProject{}, workspacesource.ErrInvalidCreateProject
	}
	var stylePresetID *uuid.UUID
	if saved.StylePresetID != uuid.Nil {
		id := saved.StylePresetID
		stylePresetID = &id
	}
	return workspacesource.CreatedProject{
		ID: saved.ID, OrgID: saved.OrgID, CanvasID: saved.CanvasID, StyleKey: saved.StyleKey, Name: saved.Name, Description: saved.Description,
		AspectRatio: saved.AspectRatio, StyleType: saved.StyleType,
		StyleSubtype: saved.StyleSubtype, StylePresetID: stylePresetID,
		Resolution: saved.Resolution, AllowOverseasModels: saved.AllowOverseasModels,
		Status: saved.Status, Revision: saved.Revision,
		CreateTime: saved.CreateTime.UTC(), UpdateTime: saved.UpdateTime.UTC(),
	}, nil
}

func equalStyleKey(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func projectCreatedEvents(actor identityapp.Principal, project domain.Project, requestID string, occurredAt time.Time) ([]identityapp.OutboxEvent, error) {
	events, err := workspacesource.ProjectCreatedEvents(actor.ID, actor.OrgID, project, requestID, occurredAt, uuid.New(), uuid.New())
	if err != nil {
		return nil, err
	}
	out := make([]identityapp.OutboxEvent, 0, len(events))
	for _, event := range events {
		out = append(out, identityapp.OutboxEvent{ID: event.ID, Topic: event.Topic, PartitionKey: event.PartitionKey, Payload: event.Payload})
	}
	return out, nil
}
