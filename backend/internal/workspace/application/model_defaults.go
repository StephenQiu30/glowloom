package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

var (
	// ErrInvalidModelDefaults means a project default-model command is malformed.
	ErrInvalidModelDefaults = errors.New("invalid project model defaults")
	// ErrDefaultModelUnavailable means a selected model cannot serve its project's capability.
	ErrDefaultModelUnavailable = errors.New("project default model unavailable")
)

// ModelDefaults is the existing project's canonical default-model map.
type ModelDefaults struct {
	ProjectID     uuid.UUID         `json:"project_id"`
	Revision      int64             `json:"revision"`
	DefaultModels map[string]string `json:"default_models" swaggertype:"object,string"`
}

// ModelDefaultsChange carries a guarded replacement and its durable request key.
type ModelDefaultsChange struct {
	ProjectID        uuid.UUID
	ExpectedRevision int64
	DefaultModels    map[string]string
	IdempotencyKey   uuid.UUID
	RequestID        string
}

// ModelDefaultsStore rechecks live project and model rights inside its transaction.
type ModelDefaultsStore interface {
	ReadModelDefaults(context.Context, identityapp.Principal, uuid.UUID) (ModelDefaults, error)
	SaveModelDefaults(context.Context, identityapp.Principal, ModelDefaultsChange, identityapp.OutboxEvent) (ModelDefaults, error)
}

// ModelDefaultsService reads and updates project defaults through the workspace boundary.
type ModelDefaultsService struct {
	store ModelDefaultsStore
	now   func() time.Time
}

// NewModelDefaultsService injects the project store and audit clock.
func NewModelDefaultsService(store ModelDefaultsStore, now func() time.Time) *ModelDefaultsService {
	return &ModelDefaultsService{store: store, now: now}
}

// Read returns only the current actor's visible project defaults.
func (s *ModelDefaultsService) Read(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID) (ModelDefaults, error) {
	if s == nil || s.store == nil || projectID == uuid.Nil {
		return ModelDefaults{}, ErrInvalidModelDefaults
	}
	if !canManageModelDefaults(actor) {
		return ModelDefaults{}, identityapp.ErrForbidden
	}
	snapshot, err := s.store.ReadModelDefaults(ctx, actor, projectID)
	if err != nil {
		return ModelDefaults{}, fmt.Errorf("read project model defaults: %w", err)
	}
	if snapshot.ProjectID != projectID || snapshot.Revision < 1 || !validDefaultModels(snapshot.DefaultModels) {
		return ModelDefaults{}, ErrInvalidModelDefaults
	}
	return snapshot, nil
}

// Save replaces defaults atomically without changing any already frozen generation.
func (s *ModelDefaultsService) Save(ctx context.Context, actor identityapp.Principal, input ModelDefaultsChange) (ModelDefaults, error) {
	if s == nil || s.store == nil || s.now == nil {
		return ModelDefaults{}, ErrInvalidModelDefaults
	}
	if !canManageModelDefaults(actor) {
		return ModelDefaults{}, identityapp.ErrForbidden
	}
	requestID, err := uuid.Parse(input.RequestID)
	if input.ProjectID == uuid.Nil || input.ExpectedRevision < 1 || input.ExpectedRevision >= math.MaxInt32 ||
		input.IdempotencyKey == uuid.Nil || err != nil || requestID == uuid.Nil || requestID.String() != input.RequestID ||
		!validDefaultModels(input.DefaultModels) {
		return ModelDefaults{}, ErrInvalidModelDefaults
	}
	input.DefaultModels = maps.Clone(input.DefaultModels)
	eventID := uuid.New()
	occurredAt := s.now().UTC()
	if occurredAt.IsZero() {
		return ModelDefaults{}, ErrInvalidModelDefaults
	}
	payload, err := json.Marshal(map[string]any{
		"event_id": eventID, "event_type": "lanverse.audit.recorded.v1", "occurred_at": occurredAt,
		"org_id": actor.OrgID, "project_id": input.ProjectID,
		"actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": "audit", "id": eventID},
		"data": map[string]any{"action": "project.defaults_changed", "object": map[string]any{"type": "project", "id": input.ProjectID},
			"request_id": input.RequestID, "before": map[string]any{"revision": input.ExpectedRevision},
			"after": map[string]any{"revision": input.ExpectedRevision + 1, "default_models_changed": true, "capability_count": len(input.DefaultModels)}},
	})
	if err != nil {
		return ModelDefaults{}, fmt.Errorf("encode project defaults audit: %w", err)
	}
	saved, err := s.store.SaveModelDefaults(ctx, actor, input, identityapp.OutboxEvent{ID: eventID, Topic: "lanverse.audit.recorded.v1", PartitionKey: input.ProjectID.String(), Payload: payload})
	if err != nil {
		return ModelDefaults{}, fmt.Errorf("save project model defaults: %w", err)
	}
	if saved.ProjectID != input.ProjectID || saved.Revision != input.ExpectedRevision+1 || !maps.Equal(saved.DefaultModels, input.DefaultModels) {
		return ModelDefaults{}, ErrInvalidModelDefaults
	}
	return saved, nil
}

func canManageModelDefaults(actor identityapp.Principal) bool {
	return actor.ID != uuid.Nil && actor.OrgID != uuid.Nil && !actor.MustChangePassword &&
		(actor.Role == identitydomain.RoleAdmin || actor.Role == identitydomain.RoleProducer)
}

func validDefaultModels(defaults map[string]string) bool {
	if defaults == nil || len(defaults) > 32 {
		return false
	}
	for capability, model := range defaults {
		if capability == "" || model == "" || strings.TrimSpace(capability) != capability || strings.TrimSpace(model) != model ||
			!utf8.ValidString(capability) || !utf8.ValidString(model) || len(capability) > 128 || len(model) > 128 {
			return false
		}
	}
	return true
}
