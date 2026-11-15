package source

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"
)

// Project event topics retain the original durable public event contract.
const (
	ProjectChangedTopic = "glowloom.workspace.project_changed.v1"
	ProjectAuditTopic   = "glowloom.audit.recorded.v1"
)

// ErrInvalidCreateProject means the create command or committed result is invalid.
var ErrInvalidCreateProject = errors.New("invalid create-project command")

// ErrIdempotencyConflict means a creation key was used for another request.
var ErrIdempotencyConflict = errors.New("project idempotency key reused")

// ProjectCreationRequest identifies one public creation and its durable receipt.
type ProjectCreationRequest struct {
	Key  uuid.UUID
	Hash string
}

// CreateProjectInput contains the specifications chosen for a new project.
type CreateProjectInput struct {
	IndependentCanvas bool
	StyleKey          *string
	Name              string
	Description       string
	AspectRatio       string
	StyleType         string
	StyleSubtype      string
	StylePresetID     uuid.UUID
	RequestID         string
}

// CreatedProject is the safe application result of project registration.
type CreatedProject struct {
	CoverAssetID        *uuid.UUID `json:"cover_asset_id,omitempty"`
	ID                  uuid.UUID  `json:"id"`
	OrgID               uuid.UUID  `json:"org_id"`
	CanvasID            uuid.UUID  `json:"canvas_id" binding:"required"`
	StyleKey            *string    `json:"style_key" extensions:"x-nullable"`
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

// ProjectCreationSHA256 retains the original public route/input fingerprint.
// Audit request IDs, authority epochs and later normalization are not logical input.
func ProjectCreationSHA256(orgID, scopeID uuid.UUID, input CreateProjectInput) (string, error) {
	body, err := json.Marshal(struct {
		OrgID         uuid.UUID
		ScopeID       uuid.UUID
		Name          string
		Description   string
		AspectRatio   string
		StyleType     string
		StyleSubtype  string
		StylePresetID uuid.UUID
		StyleKey      *string
	}{orgID, scopeID, input.Name, input.Description, input.AspectRatio, input.StyleType, input.StyleSubtype, input.StylePresetID, input.StyleKey})
	if err != nil {
		return "", fmt.Errorf("encode project creation request: %w", err)
	}
	contract := "POST /api/projects\x00"
	if input.IndependentCanvas {
		contract = "POST /api/independent-canvases\x00"
	}
	hash := sha256.Sum256(append([]byte(contract), body...))
	return hex.EncodeToString(hash[:]), nil
}

// ProjectCreationEvent is the Creation-owned durable event payload.
type ProjectCreationEvent struct {
	ID           uuid.UUID
	Topic        string
	PartitionKey string
	Payload      json.RawMessage
}

// ProjectCreatedEvents is the single original event encoder for legacy and native
// creation. Stable IDs let the native final transaction retain exact event identity.
func ProjectCreatedEvents(actorID, orgID uuid.UUID, project domain.Project, requestID string, occurredAt time.Time, changeID, auditID uuid.UUID) ([]ProjectCreationEvent, error) {
	change, err := json.Marshal(map[string]any{
		"event_id": changeID, "event_type": ProjectChangedTopic,
		"occurred_at": occurredAt, "org_id": orgID, "project_id": project.ID,
		"actor":     map[string]any{"kind": "user", "id": actorID},
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
	if project.StyleKey != nil {
		after["style_key"] = *project.StyleKey
	}
	if project.StyleSubtype != "" {
		after["style_subtype"] = project.StyleSubtype
	}
	if project.StylePresetID != uuid.Nil {
		after["style_preset_id"] = project.StylePresetID
	}
	audit, err := json.Marshal(map[string]any{
		"event_id": auditID, "event_type": ProjectAuditTopic,
		"occurred_at": occurredAt, "org_id": orgID, "project_id": project.ID,
		"actor":     map[string]any{"kind": "user", "id": actorID},
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
	return []ProjectCreationEvent{
		{ID: changeID, Topic: ProjectChangedTopic, PartitionKey: key, Payload: change},
		{ID: auditID, Topic: ProjectAuditTopic, PartitionKey: key, Payload: audit},
	}, nil
}

// ErrScopeNotFound deliberately hides private scope existence.
var ErrScopeNotFound = errors.New("creation scope not found")

var (
	// ErrProjectNotFound hides projects outside the caller's current organization.
	ErrProjectNotFound = errors.New("project not found")
)

var (
	// ErrStylePresetNotFound hides unavailable or out-of-scope presets.
	ErrStylePresetNotFound = errors.New("style preset not found")
)

var (
	// ErrStylePresetMismatch means a visible preset has different specifications.
	ErrStylePresetMismatch = errors.New("style preset does not match project style")
)

var (
	// ErrProjectDependencyUnavailable means required lifecycle evidence cannot be read.
	ErrProjectDependencyUnavailable = errors.New("project lifecycle dependency unavailable")
)
