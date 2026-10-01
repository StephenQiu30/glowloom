package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// ErrInvalidPublishModelVersion means a version publication request is incomplete.
var ErrInvalidPublishModelVersion = errors.New("invalid publish-model-version command")

// ModelVersionValidator checks the immutable configuration before publication.
type ModelVersionValidator interface {
	Validate(domain.ModelVersion) error
}

// PublishModelVersionStore commits the version, model pointer, and audit atomically.
type PublishModelVersionStore interface {
	PublishModelVersionWithAudit(context.Context, uuid.UUID, uuid.UUID, domain.ModelVersion, int64, identityapp.OutboxEvent) (domain.ModelVersion, error)
}

// PublishModelVersionInput is the reviewed configuration for the next version.
type PublishModelVersionInput struct {
	ModelID          uuid.UUID
	ExpectedRevision int64
	VersionNo        int
	ProviderModelID  string
	Modes            []string
	Limits           json.RawMessage
	ParamSchema      json.RawMessage
	SupportsQuery    bool
	SupportsCancel   bool
	SupportsCallback bool
	ExpectedMaxMS    int
	Moderation       domain.Moderation
	Queue            string
	RequestID        string
}

// PublishedModelVersion contains only the identifiers needed after publication.
type PublishedModelVersion struct {
	ID         uuid.UUID `json:"id"`
	ModelID    uuid.UUID `json:"model_id"`
	VersionNo  int       `json:"version_no"`
	Revision   int64     `json:"revision"`
	CreateTime time.Time `json:"create_time"`
}

// PublishModelVersionCommand validates and publishes immutable model configuration.
type PublishModelVersionCommand struct {
	store     PublishModelVersionStore
	validator ModelVersionValidator
	now       func() time.Time
}

// NewPublishModelVersionCommand injects storage, configuration validation, and time.
func NewPublishModelVersionCommand(store PublishModelVersionStore, validator ModelVersionValidator, now func() time.Time) *PublishModelVersionCommand {
	return &PublishModelVersionCommand{store: store, validator: validator, now: now}
}

// Execute rejects invalid configurations before requesting an atomic publication.
func (c *PublishModelVersionCommand) Execute(ctx context.Context, actor identityapp.Principal, input PublishModelVersionInput) (PublishedModelVersion, error) {
	if c == nil || c.store == nil || c.validator == nil || c.now == nil {
		return PublishedModelVersion{}, ErrInvalidPublishModelVersion
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.Role != identitydomain.RoleAdmin || actor.MustChangePassword {
		return PublishedModelVersion{}, identityapp.ErrForbidden
	}
	requestID, requestErr := uuid.Parse(input.RequestID)
	if requestErr != nil || requestID == uuid.Nil || requestID.String() != input.RequestID ||
		input.ExpectedRevision < 1 || input.ExpectedRevision >= math.MaxInt32 ||
		strings.TrimSpace(input.ProviderModelID) == "" ||
		!utf8.ValidString(input.ProviderModelID) || utf8.RuneCountInString(input.ProviderModelID) > 256 {
		return PublishedModelVersion{}, ErrInvalidPublishModelVersion
	}
	version := domain.ModelVersion{
		ID: uuid.New(), ModelID: input.ModelID, VersionNo: input.VersionNo,
		ProviderModelID: input.ProviderModelID, Modes: slices.Clone(input.Modes),
		Limits: slices.Clone(input.Limits), ParamSchema: slices.Clone(input.ParamSchema),
		SupportsQuery: input.SupportsQuery, SupportsCancel: input.SupportsCancel,
		SupportsCallback: input.SupportsCallback, ExpectedMaxMS: input.ExpectedMaxMS,
		Moderation: input.Moderation, Queue: input.Queue, CreateBy: actor.ID,
	}
	if err := version.Validate(); err != nil {
		return PublishedModelVersion{}, fmt.Errorf("validate version metadata: %w", err)
	}
	if err := c.validator.Validate(version); err != nil {
		return PublishedModelVersion{}, fmt.Errorf("validate version configuration: %w: %w", ErrInvalidPublishModelVersion, err)
	}
	occurredAt := c.now().UTC()
	if occurredAt.IsZero() {
		return PublishedModelVersion{}, ErrInvalidPublishModelVersion
	}
	event, err := modelVersionPublishedAudit(actor, version, input.ExpectedRevision+1, input.RequestID, occurredAt)
	if err != nil {
		return PublishedModelVersion{}, fmt.Errorf("build model version audit: %w", err)
	}
	saved, err := c.store.PublishModelVersionWithAudit(ctx, actor.ID, actor.OrgID, version, input.ExpectedRevision, event)
	if err != nil {
		return PublishedModelVersion{}, fmt.Errorf("publish model version with audit: %w", err)
	}
	if saved.ID != version.ID || saved.ModelID != version.ModelID ||
		saved.VersionNo != version.VersionNo || saved.ProviderModelID != version.ProviderModelID ||
		!slices.Equal(saved.Modes, version.Modes) || saved.CreateBy != actor.ID || saved.CreateTime.IsZero() {
		return PublishedModelVersion{}, ErrInvalidPublishModelVersion
	}
	return PublishedModelVersion{
		ID: saved.ID, ModelID: saved.ModelID, VersionNo: saved.VersionNo,
		Revision: input.ExpectedRevision + 1, CreateTime: saved.CreateTime,
	}, nil
}

func modelVersionPublishedAudit(actor identityapp.Principal, version domain.ModelVersion, revision int64, requestID string, occurredAt time.Time) (identityapp.OutboxEvent, error) {
	eventID := uuid.New()
	payload, err := json.Marshal(map[string]any{
		"event_id": eventID, "event_type": auditTopic,
		"occurred_at": occurredAt, "org_id": actor.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "audit", "id": eventID},
		"data": map[string]any{
			"action":     "model.version_published",
			"object":     map[string]any{"type": "model_profile", "id": version.ModelID},
			"request_id": requestID,
			"after": map[string]any{
				"version_id": version.ID, "version_no": version.VersionNo,
				"provider_model_id": version.ProviderModelID, "revision": revision,
			},
		},
	})
	if err != nil {
		return identityapp.OutboxEvent{}, fmt.Errorf("encode model version audit: %w", err)
	}
	return identityapp.OutboxEvent{
		ID: eventID, Topic: auditTopic, PartitionKey: actor.OrgID.String(), Payload: payload,
	}, nil
}
