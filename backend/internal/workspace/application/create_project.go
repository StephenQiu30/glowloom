// Package application coordinates project commands across domain and persistence.
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

const (
	projectChangedTopic = "lanverse.workspace.project_changed.v1"
	projectAuditTopic   = "lanverse.audit.recorded.v1"
)

// ErrInvalidCreateProject means the create command or committed result is invalid.
var ErrInvalidCreateProject = errors.New("invalid create-project command")

// ErrIdempotencyConflict means an active key was used for another request.
var ErrIdempotencyConflict = errors.New("project idempotency key reused")

// CreateProjectStore rechecks current rights and commits the project, its zero
// budget, and both durable events in one transaction.
type CreateProjectStore interface {
	CreateProjectWithEvents(context.Context, identityapp.Principal, domain.Project, []identityapp.OutboxEvent) (domain.Project, error)
	CreateProjectIdempotently(context.Context, identityapp.Principal, domain.Project, []identityapp.OutboxEvent, ProjectCreationRequest) (domain.Project, error)
}

// ProjectCreationRequest identifies one public creation and its bounded receipt.
type ProjectCreationRequest struct {
	Key  uuid.UUID
	Hash string
}

// CreateProjectInput contains the specifications chosen for a new project.
type CreateProjectInput struct {
	Name          string
	Description   string
	AspectRatio   string
	StyleType     string
	StyleSubtype  string
	StylePresetID uuid.UUID
	RequestID     string
}

// CreatedProject is the safe application result of project registration.
type CreatedProject struct {
	CoverAssetID        *uuid.UUID `json:"cover_asset_id,omitempty"`
	ID                  uuid.UUID  `json:"id"`
	OrgID               uuid.UUID  `json:"org_id"`
	Name                string     `json:"name"`
	Description         string     `json:"description"`
	AspectRatio         string     `json:"aspect_ratio"`
	StyleType           string     `json:"style_type"`
	StyleSubtype        string     `json:"style_subtype,omitempty"`
	StylePresetID       *uuid.UUID `json:"style_preset_id,omitempty"`
	Resolution          string     `json:"resolution"`
	AllowOverseasModels bool       `json:"allow_overseas_models"`
	Status              string     `json:"status"`
	Revision            int64      `json:"revision"`
	CreateTime          time.Time  `json:"create_time"`
	UpdateTime          time.Time  `json:"update_time"`
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
func (c *CreateProjectCommand) Execute(ctx context.Context, actor identityapp.Principal, input CreateProjectInput) (CreatedProject, error) {
	return c.execute(ctx, actor, input, nil)
}

// ExecuteIdempotent is the public creation entry point. The request identifier
// belongs to the audit; it does not change the logical request fingerprint.
func (c *CreateProjectCommand) ExecuteIdempotent(ctx context.Context, actor identityapp.Principal, key uuid.UUID, input CreateProjectInput) (CreatedProject, error) {
	if key == uuid.Nil {
		return CreatedProject{}, ErrInvalidCreateProject
	}
	body, err := json.Marshal(struct {
		OrgID         uuid.UUID
		Name          string
		Description   string
		AspectRatio   string
		StyleType     string
		StyleSubtype  string
		StylePresetID uuid.UUID
	}{actor.OrgID, input.Name, input.Description, input.AspectRatio, input.StyleType, input.StyleSubtype, input.StylePresetID})
	if err != nil {
		return CreatedProject{}, fmt.Errorf("encode project creation request: %w", err)
	}
	hash := sha256.Sum256(append([]byte("POST /api/projects\x00"), body...))
	return c.execute(ctx, actor, input, &ProjectCreationRequest{Key: key, Hash: hex.EncodeToString(hash[:])})
}

func (c *CreateProjectCommand) execute(ctx context.Context, actor identityapp.Principal, input CreateProjectInput, request *ProjectCreationRequest) (CreatedProject, error) {
	if c == nil || c.store == nil || c.now == nil {
		return CreatedProject{}, ErrInvalidCreateProject
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.MustChangePassword ||
		(actor.Role != identitydomain.RoleAdmin && actor.Role != identitydomain.RoleProducer) {
		return CreatedProject{}, identityapp.ErrForbidden
	}
	requestID, err := uuid.Parse(input.RequestID)
	name := strings.TrimSpace(input.Name)
	if err != nil || requestID == uuid.Nil || requestID.String() != input.RequestID ||
		!utf8.ValidString(name) || !utf8.ValidString(input.Description) {
		return CreatedProject{}, ErrInvalidCreateProject
	}
	project := domain.Project{
		ID: uuid.New(), OrgID: actor.OrgID, Name: name, Description: input.Description,
		AspectRatio: input.AspectRatio, StyleType: input.StyleType,
		StyleSubtype: input.StyleSubtype, StylePresetID: input.StylePresetID,
		Resolution: "1080p", Status: "active", Revision: 1,
	}
	if err := project.Validate(); err != nil {
		return CreatedProject{}, ErrInvalidCreateProject
	}
	occurredAt := c.now().UTC()
	if occurredAt.IsZero() {
		return CreatedProject{}, ErrInvalidCreateProject
	}
	events, err := projectCreatedEvents(actor, project, input.RequestID, occurredAt)
	if err != nil {
		return CreatedProject{}, fmt.Errorf("build project events: %w", err)
	}
	var saved domain.Project
	if request == nil {
		saved, err = c.store.CreateProjectWithEvents(ctx, actor, project, events)
	} else {
		saved, err = c.store.CreateProjectIdempotently(ctx, actor, project, events, *request)
	}
	if err != nil {
		return CreatedProject{}, fmt.Errorf("create project with events: %w", err)
	}
	if saved.ID == uuid.Nil || (request == nil && saved.ID != project.ID) || saved.OrgID != project.OrgID || saved.Name != project.Name ||
		saved.Description != project.Description || saved.AspectRatio != project.AspectRatio ||
		saved.StyleType != project.StyleType || saved.StyleSubtype != project.StyleSubtype ||
		saved.StylePresetID != project.StylePresetID || saved.Resolution != "1080p" ||
		saved.AllowOverseasModels || saved.Status != "active" || saved.Revision != 1 ||
		saved.IsDelete || saved.ArchivedAt != nil || saved.DeleteTime != nil ||
		saved.PurgeAfter != nil || saved.CreateTime.IsZero() || saved.UpdateTime.IsZero() {
		return CreatedProject{}, ErrInvalidCreateProject
	}
	var stylePresetID *uuid.UUID
	if saved.StylePresetID != uuid.Nil {
		id := saved.StylePresetID
		stylePresetID = &id
	}
	return CreatedProject{
		ID: saved.ID, OrgID: saved.OrgID, Name: saved.Name, Description: saved.Description,
		AspectRatio: saved.AspectRatio, StyleType: saved.StyleType,
		StyleSubtype: saved.StyleSubtype, StylePresetID: stylePresetID,
		Resolution: saved.Resolution, AllowOverseasModels: saved.AllowOverseasModels,
		Status: saved.Status, Revision: saved.Revision,
		CreateTime: saved.CreateTime.UTC(), UpdateTime: saved.UpdateTime.UTC(),
	}, nil
}

func projectCreatedEvents(actor identityapp.Principal, project domain.Project, requestID string, occurredAt time.Time) ([]identityapp.OutboxEvent, error) {
	changeID, auditID := uuid.New(), uuid.New()
	change, err := json.Marshal(map[string]any{
		"event_id": changeID, "event_type": projectChangedTopic,
		"occurred_at": occurredAt, "org_id": actor.OrgID, "project_id": project.ID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "project", "id": project.ID, "revision": project.Revision},
		"data":      map[string]any{"change": "created"},
	})
	if err != nil {
		return nil, fmt.Errorf("encode project changed event: %w", err)
	}
	after := map[string]any{
		"aspect_ratio": project.AspectRatio, "style_type": project.StyleType,
		"status": project.Status, "revision": project.Revision,
	}
	if project.StyleSubtype != "" {
		after["style_subtype"] = project.StyleSubtype
	}
	if project.StylePresetID != uuid.Nil {
		after["style_preset_id"] = project.StylePresetID
	}
	audit, err := json.Marshal(map[string]any{
		"event_id": auditID, "event_type": projectAuditTopic,
		"occurred_at": occurredAt, "org_id": actor.OrgID, "project_id": project.ID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "audit", "id": auditID},
		"data": map[string]any{
			"action": "project.created", "object": map[string]any{"type": "project", "id": project.ID},
			"request_id": requestID, "after": after,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode project created audit: %w", err)
	}
	key := project.ID.String()
	return []identityapp.OutboxEvent{
		{ID: changeID, Topic: projectChangedTopic, PartitionKey: key, Payload: change},
		{ID: auditID, Topic: projectAuditTopic, PartitionKey: key, Payload: audit},
	}, nil
}
