package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// ErrInvalidPublishPriceRule means a price publication request is incomplete.
var ErrInvalidPublishPriceRule = errors.New("invalid publish-price-rule command")

// PriceRuleValidator checks the immutable pricing configuration before publication.
type PriceRuleValidator interface {
	Validate(domain.PriceRuleVersion) error
}

// PublishPriceRuleStore commits the price, model revision, and audit atomically.
type PublishPriceRuleStore interface {
	PublishPriceRuleWithAudit(context.Context, uuid.UUID, uuid.UUID, domain.PriceRuleVersion, int64, identityapp.OutboxEvent) (domain.PriceRuleVersion, error)
}

// PublishPriceRuleInput is the reviewed configuration for the next price version.
type PublishPriceRuleInput struct {
	ModelID          uuid.UUID
	ExpectedRevision int64
	VersionNo        int
	Unit             domain.PriceUnit
	Rule             json.RawMessage
	Currency         string
	FXRateToCNY      string
	EffectiveFrom    time.Time
	RequestID        string
}

// PublishedPriceRule contains only the identifiers needed after publication.
type PublishedPriceRule struct {
	ID         uuid.UUID `json:"id"`
	ModelID    uuid.UUID `json:"model_id"`
	VersionNo  int       `json:"version_no"`
	Revision   int64     `json:"revision"`
	CreateTime time.Time `json:"create_time"`
}

// PublishPriceRuleCommand validates and publishes an immutable price rule.
type PublishPriceRuleCommand struct {
	store     PublishPriceRuleStore
	validator PriceRuleValidator
	now       func() time.Time
}

// NewPublishPriceRuleCommand injects storage, configuration validation, and time.
func NewPublishPriceRuleCommand(store PublishPriceRuleStore, validator PriceRuleValidator, now func() time.Time) *PublishPriceRuleCommand {
	return &PublishPriceRuleCommand{store: store, validator: validator, now: now}
}

// Execute rejects invalid rules before requesting an atomic publication.
func (c *PublishPriceRuleCommand) Execute(ctx context.Context, actor identityapp.Principal, input PublishPriceRuleInput) (PublishedPriceRule, error) {
	if c == nil || c.store == nil || c.validator == nil || c.now == nil {
		return PublishedPriceRule{}, ErrInvalidPublishPriceRule
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.Role != identitydomain.RoleAdmin || actor.MustChangePassword {
		return PublishedPriceRule{}, identityapp.ErrForbidden
	}
	requestID, err := uuid.Parse(input.RequestID)
	if err != nil || requestID == uuid.Nil || requestID.String() != input.RequestID ||
		input.ExpectedRevision < 1 || input.ExpectedRevision >= math.MaxInt32 {
		return PublishedPriceRule{}, ErrInvalidPublishPriceRule
	}
	price := domain.PriceRuleVersion{
		ID: uuid.New(), ModelID: input.ModelID, VersionNo: input.VersionNo,
		Unit: input.Unit, Rule: slices.Clone(input.Rule), Currency: input.Currency,
		FXRateToCNY: input.FXRateToCNY, EffectiveFrom: input.EffectiveFrom.UTC().Round(time.Microsecond),
		CreateBy: actor.ID,
	}
	if err := price.Validate(); err != nil {
		return PublishedPriceRule{}, fmt.Errorf("validate price metadata: %w", err)
	}
	if err := c.validator.Validate(price); err != nil {
		return PublishedPriceRule{}, fmt.Errorf("validate price configuration: %w", err)
	}
	occurredAt := c.now().UTC()
	if occurredAt.IsZero() {
		return PublishedPriceRule{}, ErrInvalidPublishPriceRule
	}
	event, err := priceRulePublishedAudit(actor, price, input.ExpectedRevision+1, input.RequestID, occurredAt)
	if err != nil {
		return PublishedPriceRule{}, fmt.Errorf("build price audit: %w", err)
	}
	saved, err := c.store.PublishPriceRuleWithAudit(ctx, actor.ID, actor.OrgID, price, input.ExpectedRevision, event)
	if err != nil {
		return PublishedPriceRule{}, fmt.Errorf("publish price with audit: %w", err)
	}
	if saved.ID != price.ID || saved.ModelID != price.ModelID ||
		saved.VersionNo != price.VersionNo || saved.Unit != price.Unit ||
		!slices.Equal(saved.Rule, price.Rule) || saved.Currency != price.Currency ||
		saved.FXRateToCNY != price.FXRateToCNY ||
		!saved.EffectiveFrom.Equal(price.EffectiveFrom) || saved.CreateBy != actor.ID ||
		saved.CreateTime.IsZero() {
		return PublishedPriceRule{}, ErrInvalidPublishPriceRule
	}
	return PublishedPriceRule{
		ID: saved.ID, ModelID: saved.ModelID, VersionNo: saved.VersionNo,
		Revision: input.ExpectedRevision + 1, CreateTime: saved.CreateTime,
	}, nil
}

func priceRulePublishedAudit(actor identityapp.Principal, price domain.PriceRuleVersion, revision int64, requestID string, occurredAt time.Time) (identityapp.OutboxEvent, error) {
	eventID := uuid.New()
	payload, err := json.Marshal(map[string]any{
		"event_id": eventID, "event_type": auditTopic,
		"occurred_at": occurredAt, "org_id": actor.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "audit", "id": eventID},
		"data": map[string]any{
			"action":     "price.published",
			"object":     map[string]any{"type": "model_profile", "id": price.ModelID},
			"request_id": requestID,
			"after": map[string]any{
				"version_id": price.ID, "version_no": price.VersionNo,
				"unit": price.Unit, "currency": price.Currency,
				"effective_from": price.EffectiveFrom, "revision": revision,
			},
		},
	})
	if err != nil {
		return identityapp.OutboxEvent{}, fmt.Errorf("encode price audit: %w", err)
	}
	return identityapp.OutboxEvent{
		ID: eventID, Topic: auditTopic, PartitionKey: actor.OrgID.String(), Payload: payload,
	}, nil
}
