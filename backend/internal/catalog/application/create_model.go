package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// ErrInvalidCreateModel means a model registration or its committed result is invalid.
var ErrInvalidCreateModel = errors.New("invalid create-model command")

// CreateModelStore commits a disabled model and its audit in one transaction.
type CreateModelStore interface {
	CreateModelWithAudit(context.Context, uuid.UUID, uuid.UUID, domain.ModelProfile, identityapp.OutboxEvent) (domain.ModelProfile, error)
}

// CreateModelInput identifies the provider and capability of a new model.
type CreateModelInput struct {
	Key         string
	ProviderID  uuid.UUID
	Capability  string
	DisplayName string
	RequestID   string
}

// CreatedModel is the safe registration summary.
type CreatedModel struct {
	ID               uuid.UUID          `json:"id"`
	Key              string             `json:"model_key"`
	ProviderID       uuid.UUID          `json:"provider_id"`
	Capability       string             `json:"capability"`
	DisplayName      string             `json:"display_name"`
	Status           domain.ModelStatus `json:"status"`
	CurrentVersionID uuid.UUID          `json:"current_version_id"`
	Revision         int64              `json:"revision"`
	CreateTime       time.Time          `json:"create_time"`
	UpdateTime       time.Time          `json:"update_time"`
}

// CreateModelCommand registers a disabled model with durable audit.
type CreateModelCommand struct {
	store CreateModelStore
	now   func() time.Time
}

// NewCreateModelCommand injects persistence and a clock.
func NewCreateModelCommand(store CreateModelStore, now func() time.Time) *CreateModelCommand {
	return &CreateModelCommand{store: store, now: now}
}

// Execute validates identity and input before committing the registration.
func (c *CreateModelCommand) Execute(ctx context.Context, actor identityapp.Principal, input CreateModelInput) (CreatedModel, error) {
	if c == nil || c.store == nil || c.now == nil {
		return CreatedModel{}, ErrInvalidCreateModel
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.Role != identitydomain.RoleAdmin || actor.MustChangePassword {
		return CreatedModel{}, identityapp.ErrForbidden
	}
	requestID, err := uuid.Parse(input.RequestID)
	name := strings.TrimSpace(input.DisplayName)
	if err != nil || requestID == uuid.Nil || requestID.String() != input.RequestID ||
		input.ProviderID == uuid.Nil || !validModelKey(input.Key) ||
		strings.TrimSpace(input.Capability) == "" || len(input.Capability) > 128 ||
		name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 100 {
		return CreatedModel{}, ErrInvalidCreateModel
	}
	model := domain.ModelProfile{
		ID: uuid.New(), Key: input.Key, ProviderID: input.ProviderID,
		Capability: input.Capability, DisplayName: name,
		Status: domain.ModelDisabled, Revision: 1,
	}
	if err := model.Validate(); err != nil {
		return CreatedModel{}, err
	}
	occurredAt := c.now().UTC()
	if occurredAt.IsZero() {
		return CreatedModel{}, ErrInvalidCreateModel
	}
	event, err := modelCreatedAudit(actor, model, input.RequestID, occurredAt)
	if err != nil {
		return CreatedModel{}, fmt.Errorf("build model created audit: %w", err)
	}
	saved, err := c.store.CreateModelWithAudit(ctx, actor.ID, actor.OrgID, model, event)
	if err != nil {
		return CreatedModel{}, fmt.Errorf("create model with audit: %w", err)
	}
	if saved.ID != model.ID || saved.Key != model.Key || saved.ProviderID != model.ProviderID ||
		saved.Capability != model.Capability || saved.DisplayName != model.DisplayName ||
		saved.Status != domain.ModelDisabled || saved.CurrentVersionID != uuid.Nil ||
		saved.Revision != 1 || saved.CreateTime.IsZero() || saved.UpdateTime.IsZero() {
		return CreatedModel{}, ErrInvalidCreateModel
	}
	return CreatedModel{
		ID: saved.ID, Key: saved.Key, ProviderID: saved.ProviderID,
		Capability: saved.Capability, DisplayName: saved.DisplayName,
		Status: saved.Status, CurrentVersionID: saved.CurrentVersionID,
		Revision: saved.Revision, CreateTime: saved.CreateTime, UpdateTime: saved.UpdateTime,
	}, nil
}

func validModelKey(key string) bool {
	if len(key) < 1 || len(key) > 128 || key[0] < 'a' || key[0] > 'z' {
		return false
	}
	for index := 1; index < len(key); index++ {
		char := key[index]
		if char < 'a' || char > 'z' {
			if char < '0' || char > '9' {
				if char != '-' && char != '_' && char != '.' {
					return false
				}
			}
		}
	}
	return true
}

func modelCreatedAudit(actor identityapp.Principal, model domain.ModelProfile, requestID string, occurredAt time.Time) (identityapp.OutboxEvent, error) {
	eventID := uuid.New()
	payload, err := json.Marshal(map[string]any{
		"event_id": eventID, "event_type": auditTopic,
		"occurred_at": occurredAt, "org_id": actor.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "audit", "id": eventID},
		"data": map[string]any{
			"action":     "model.created",
			"object":     map[string]any{"type": "model_profile", "id": model.ID},
			"request_id": requestID,
			"after": map[string]any{
				"key": model.Key, "provider_id": model.ProviderID,
				"capability": model.Capability, "display_name": model.DisplayName,
				"status": model.Status,
			},
		},
	})
	if err != nil {
		return identityapp.OutboxEvent{}, fmt.Errorf("encode model created audit: %w", err)
	}
	return identityapp.OutboxEvent{
		ID: eventID, Topic: auditTopic, PartitionKey: actor.OrgID.String(), Payload: payload,
	}, nil
}
