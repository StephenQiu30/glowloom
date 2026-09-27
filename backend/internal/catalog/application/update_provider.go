package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// ErrInvalidUpdateProvider means the patch or committed result is invalid.
var ErrInvalidUpdateProvider = errors.New("invalid update-provider command")

// UpdateProviderStore reads current settings and commits a guarded change with events.
type UpdateProviderStore interface {
	FindProviderForAdmin(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (domain.Provider, error)
	UpdateProviderWithEvents(context.Context, uuid.UUID, uuid.UUID, domain.Provider, domain.Provider, []identityapp.OutboxEvent) (domain.Provider, error)
}

// UpdateProviderInput includes only mutable operational settings.
type UpdateProviderInput struct {
	ProviderID       uuid.UUID
	ExpectedRevision int64
	Status           *domain.ProviderStatus
	ConcurrencyLimit *int
	RateLimitPerMin  *int
	RequestID        string
}

// UpdateProviderCommand changes operational settings with a revision and durable audit.
type UpdateProviderCommand struct {
	store UpdateProviderStore
	now   func() time.Time
}

// NewUpdateProviderCommand injects storage and a clock.
func NewUpdateProviderCommand(store UpdateProviderStore, now func() time.Time) *UpdateProviderCommand {
	return &UpdateProviderCommand{store: store, now: now}
}

// Execute rejects empty or stale patches and returns only provider settings.
func (c *UpdateProviderCommand) Execute(ctx context.Context, actor identityapp.Principal, input UpdateProviderInput) (CreatedProvider, error) {
	if c == nil || c.store == nil || c.now == nil {
		return CreatedProvider{}, ErrInvalidUpdateProvider
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.Role != identitydomain.RoleAdmin || actor.MustChangePassword {
		return CreatedProvider{}, identityapp.ErrForbidden
	}
	requestID, err := uuid.Parse(input.RequestID)
	if input.ProviderID == uuid.Nil || input.ExpectedRevision < 1 || input.ExpectedRevision >= math.MaxInt32 ||
		err != nil || requestID == uuid.Nil || requestID.String() != input.RequestID ||
		(input.Status == nil && input.ConcurrencyLimit == nil && input.RateLimitPerMin == nil) ||
		(input.Status != nil && *input.Status != domain.ProviderActive && *input.Status != domain.ProviderDisabled) ||
		(input.ConcurrencyLimit != nil && (*input.ConcurrencyLimit < 1 || *input.ConcurrencyLimit > math.MaxInt32)) ||
		(input.RateLimitPerMin != nil && (*input.RateLimitPerMin < 1 || *input.RateLimitPerMin > math.MaxInt32)) {
		return CreatedProvider{}, ErrInvalidUpdateProvider
	}
	before, err := c.store.FindProviderForAdmin(ctx, actor.ID, actor.OrgID, input.ProviderID)
	if err != nil {
		return CreatedProvider{}, fmt.Errorf("find provider to update: %w", err)
	}
	if before.ID != input.ProviderID || before.Validate() != nil {
		return CreatedProvider{}, ErrInvalidUpdateProvider
	}
	if before.Revision != input.ExpectedRevision {
		return CreatedProvider{}, domain.ErrProviderRevisionConflict
	}
	after := before
	if input.Status != nil {
		after.Status = *input.Status
	}
	if input.ConcurrencyLimit != nil {
		after.ConcurrencyLimit = *input.ConcurrencyLimit
	}
	if input.RateLimitPerMin != nil {
		after.RateLimitPerMin = *input.RateLimitPerMin
	}
	if after.Status == before.Status && after.ConcurrencyLimit == before.ConcurrencyLimit &&
		after.RateLimitPerMin == before.RateLimitPerMin {
		return CreatedProvider{}, ErrInvalidUpdateProvider
	}
	after.Revision++
	if err := after.Validate(); err != nil {
		return CreatedProvider{}, fmt.Errorf("validate provider update: %w", err)
	}
	occurredAt := c.now().UTC()
	if occurredAt.IsZero() {
		return CreatedProvider{}, ErrInvalidUpdateProvider
	}
	events, err := providerUpdatedEvents(actor, before, after, input.RequestID, occurredAt)
	if err != nil {
		return CreatedProvider{}, fmt.Errorf("build provider update events: %w", err)
	}
	saved, err := c.store.UpdateProviderWithEvents(ctx, actor.ID, actor.OrgID, before, after, events)
	if err != nil {
		return CreatedProvider{}, fmt.Errorf("update provider with events: %w", err)
	}
	if saved.ID != after.ID || saved.Key != after.Key || saved.Name != after.Name ||
		saved.AdapterKey != after.AdapterKey || saved.Region != after.Region ||
		saved.Status != after.Status || saved.ConcurrencyLimit != after.ConcurrencyLimit ||
		saved.RateLimitPerMin != after.RateLimitPerMin || saved.Revision != after.Revision ||
		saved.CreateTime.IsZero() || saved.UpdateTime.IsZero() {
		return CreatedProvider{}, ErrInvalidUpdateProvider
	}
	return CreatedProvider{
		ID: saved.ID, Key: saved.Key, Name: saved.Name, AdapterKey: saved.AdapterKey,
		Region: saved.Region, Status: saved.Status, ConcurrencyLimit: saved.ConcurrencyLimit,
		RateLimitPerMin: saved.RateLimitPerMin, Revision: saved.Revision,
		CreateTime: saved.CreateTime, UpdateTime: saved.UpdateTime,
	}, nil
}

func providerUpdatedEvents(actor identityapp.Principal, before, after domain.Provider, requestID string, occurredAt time.Time) ([]identityapp.OutboxEvent, error) {
	changedID, auditID := uuid.New(), uuid.New()
	changed, err := json.Marshal(map[string]any{
		"event_id": changedID, "event_type": "lanverse.catalog.provider_changed.v1",
		"occurred_at": occurredAt, "org_id": actor.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "provider", "id": after.ID},
		"data":      map[string]any{"provider_id": after.ID, "revision": after.Revision},
	})
	if err != nil {
		return nil, fmt.Errorf("encode provider change: %w", err)
	}
	summary := func(p domain.Provider) map[string]any {
		return map[string]any{
			"status": p.Status, "concurrency_limit": p.ConcurrencyLimit,
			"rate_limit_per_min": p.RateLimitPerMin,
		}
	}
	audit, err := json.Marshal(map[string]any{
		"event_id": auditID, "event_type": auditTopic,
		"occurred_at": occurredAt, "org_id": actor.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "audit", "id": auditID},
		"data": map[string]any{
			"action": "provider.updated", "object": map[string]any{"type": "provider", "id": after.ID},
			"request_id": requestID, "before": summary(before), "after": summary(after),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode provider audit: %w", err)
	}
	key := actor.OrgID.String()
	return []identityapp.OutboxEvent{
		{ID: changedID, Topic: "lanverse.catalog.provider_changed.v1", PartitionKey: key, Payload: changed},
		{ID: auditID, Topic: auditTopic, PartitionKey: key, Payload: audit},
	}, nil
}
