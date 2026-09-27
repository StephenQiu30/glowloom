package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// ErrInvalidCreateProvider means a provider request or committed result is invalid.
var ErrInvalidCreateProvider = errors.New("invalid create-provider command")

// CreateProviderStore checks current administrator rights and commits the audit.
type CreateProviderStore interface {
	CreateProviderWithAudit(context.Context, uuid.UUID, uuid.UUID, domain.Provider, identityapp.OutboxEvent) (domain.Provider, error)
}

// ProviderAdapters declares the adapter names with supported credential fields.
type ProviderAdapters interface {
	Supports(string) bool
}

// CreateProviderInput is the administrator's provider registration request.
type CreateProviderInput struct {
	Key              string
	Name             string
	AdapterKey       string
	Region           domain.Region
	ConcurrencyLimit int
	RateLimitPerMin  int
	RequestID        string
}

// CreatedProvider is the safe summary returned by registration.
type CreatedProvider struct {
	ID               uuid.UUID             `json:"id"`
	Key              string                `json:"key"`
	Name             string                `json:"name"`
	AdapterKey       string                `json:"adapter_key"`
	Region           domain.Region         `json:"region"`
	Status           domain.ProviderStatus `json:"status"`
	ConcurrencyLimit int                   `json:"concurrency_limit"`
	RateLimitPerMin  int                   `json:"rate_limit_per_min"`
	Revision         int64                 `json:"revision"`
	CreateTime       time.Time             `json:"create_time"`
	UpdateTime       time.Time             `json:"update_time"`
}

// CreateProviderCommand registers a supported provider with durable audit.
type CreateProviderCommand struct {
	store    CreateProviderStore
	adapters ProviderAdapters
	now      func() time.Time
}

// NewCreateProviderCommand injects persistence, adapter contracts, and clock.
func NewCreateProviderCommand(store CreateProviderStore, adapters ProviderAdapters, now func() time.Time) *CreateProviderCommand {
	return &CreateProviderCommand{store: store, adapters: adapters, now: now}
}

// Execute validates the provider and commits it with a safe audit event.
func (c *CreateProviderCommand) Execute(ctx context.Context, actor identityapp.Principal, input CreateProviderInput) (CreatedProvider, error) {
	if c == nil || c.store == nil || c.adapters == nil || c.now == nil {
		return CreatedProvider{}, ErrInvalidCreateProvider
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.Role != identitydomain.RoleAdmin || actor.MustChangePassword {
		return CreatedProvider{}, identityapp.ErrForbidden
	}
	requestID, err := uuid.Parse(input.RequestID)
	name := strings.TrimSpace(input.Name)
	if err != nil || requestID == uuid.Nil || requestID.String() != input.RequestID || !validProviderKey(input.Key) ||
		name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 100 ||
		!c.adapters.Supports(input.AdapterKey) || input.ConcurrencyLimit < 1 ||
		input.ConcurrencyLimit > math.MaxInt32 || input.RateLimitPerMin < 1 || input.RateLimitPerMin > math.MaxInt32 {
		return CreatedProvider{}, ErrInvalidCreateProvider
	}
	provider := domain.Provider{
		ID: uuid.New(), Key: input.Key, Name: name, AdapterKey: input.AdapterKey,
		Region: input.Region, Status: domain.ProviderActive,
		ConcurrencyLimit: input.ConcurrencyLimit, RateLimitPerMin: input.RateLimitPerMin, Revision: 1,
	}
	if err := provider.Validate(); err != nil {
		return CreatedProvider{}, err
	}
	occurredAt := c.now().UTC()
	if occurredAt.IsZero() {
		return CreatedProvider{}, ErrInvalidCreateProvider
	}
	event, err := providerCreatedAudit(actor, provider, input.RequestID, occurredAt)
	if err != nil {
		return CreatedProvider{}, fmt.Errorf("build provider audit: %w", err)
	}
	saved, err := c.store.CreateProviderWithAudit(ctx, actor.ID, actor.OrgID, provider, event)
	if err != nil {
		return CreatedProvider{}, fmt.Errorf("create provider with audit: %w", err)
	}
	if saved.ID != provider.ID || saved.Key != provider.Key || saved.Name != provider.Name ||
		saved.AdapterKey != provider.AdapterKey || saved.Region != provider.Region ||
		saved.Status != domain.ProviderActive || saved.ConcurrencyLimit != provider.ConcurrencyLimit ||
		saved.RateLimitPerMin != provider.RateLimitPerMin || saved.Revision != 1 ||
		saved.CreateTime.IsZero() || saved.UpdateTime.IsZero() {
		return CreatedProvider{}, ErrInvalidCreateProvider
	}
	return CreatedProvider{
		ID: saved.ID, Key: saved.Key, Name: saved.Name, AdapterKey: saved.AdapterKey,
		Region: saved.Region, Status: saved.Status,
		ConcurrencyLimit: saved.ConcurrencyLimit, RateLimitPerMin: saved.RateLimitPerMin,
		Revision: saved.Revision, CreateTime: saved.CreateTime, UpdateTime: saved.UpdateTime,
	}, nil
}

func validProviderKey(key string) bool {
	if len(key) < 1 || len(key) > 64 || key[0] < 'a' || key[0] > 'z' {
		return false
	}
	for index := 1; index < len(key); index++ {
		char := key[index]
		if char < 'a' || char > 'z' {
			if char < '0' || char > '9' {
				if char != '_' && char != '-' {
					return false
				}
			}
		}
	}
	return true
}

func providerCreatedAudit(actor identityapp.Principal, provider domain.Provider, requestID string, occurredAt time.Time) (identityapp.OutboxEvent, error) {
	eventID := uuid.New()
	payload, err := json.Marshal(map[string]any{
		"event_id": eventID, "event_type": auditTopic,
		"occurred_at": occurredAt, "org_id": actor.OrgID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "audit", "id": eventID},
		"data": map[string]any{
			"action":     "provider.created",
			"object":     map[string]any{"type": "provider", "id": provider.ID},
			"request_id": requestID,
			"after": map[string]any{
				"key": provider.Key, "adapter_key": provider.AdapterKey,
				"region": provider.Region, "status": provider.Status,
				"concurrency_limit":  provider.ConcurrencyLimit,
				"rate_limit_per_min": provider.RateLimitPerMin,
			},
		},
	})
	if err != nil {
		return identityapp.OutboxEvent{}, fmt.Errorf("encode provider created audit: %w", err)
	}
	return identityapp.OutboxEvent{
		ID: eventID, Topic: auditTopic, PartitionKey: actor.OrgID.String(), Payload: payload,
	}, nil
}
