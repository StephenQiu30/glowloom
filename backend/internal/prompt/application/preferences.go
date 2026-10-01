// Package application coordinates workspace prompt preferences without changing execution rights.
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/prompt/domain"
)

var (
	// ErrRevisionConflict means the personal preference changed after it was read.
	ErrRevisionConflict = errors.New("prompt preference revision conflict")
	// ErrBaselineConflict means a request refers to a different immutable baseline.
	ErrBaselineConflict = errors.New("prompt preference baseline conflict")
	// ErrIdempotencyConflict means a workspace write reused another request's key.
	ErrIdempotencyConflict = errors.New("prompt preference idempotency conflict")
)

// Preference combines a read-only baseline and the current actor's personal layer.
type Preference struct {
	Definition    domain.Definition     `json:"definition"`
	Customization *domain.Customization `json:"customization"`
	Outdated      bool                  `json:"outdated"`
}

// SaveInput is one personal customization relative to an observed revision.
type SaveInput struct {
	Operation        string
	Mode             domain.Mode
	Content          string
	BaseTemplateID   uuid.UUID
	ExpectedRevision int64
	IdempotencyKey   uuid.UUID
	RequestID        string
}

// Store persists only the current workspace actor's preferences and durable events.
type Store interface {
	ReadCustomizations(context.Context, identityapp.Principal) ([]domain.Customization, error)
	SaveCustomization(context.Context, identityapp.Principal, SaveInput, domain.Customization, identityapp.OutboxEvent) (domain.Customization, error)
}

// Preferences reads and saves personal creative policy beneath server-owned constraints.
type Preferences struct {
	store Store
	now   func() time.Time
}

// NewPreferences injects transactional storage and the audit clock.
func NewPreferences(store Store, now func() time.Time) *Preferences {
	return &Preferences{store: store, now: now}
}

// List returns all immutable definitions even before a personal preference exists.
func (p *Preferences) List(ctx context.Context, actor identityapp.Principal) ([]Preference, error) {
	if p == nil || p.store == nil {
		return nil, domain.ErrInvalidCustomization
	}
	if !canCustomize(actor) {
		return nil, identityapp.ErrForbidden
	}
	customizations, err := p.store.ReadCustomizations(ctx, actor)
	if err != nil {
		return nil, fmt.Errorf("read workspace prompt preferences: %w", err)
	}
	byOperation := make(map[string]domain.Customization, len(customizations))
	for _, customization := range customizations {
		if customization.Validate() != nil {
			return nil, domain.ErrInvalidCustomization
		}
		if _, duplicate := byOperation[customization.Operation]; duplicate {
			return nil, domain.ErrInvalidCustomization
		}
		byOperation[customization.Operation] = customization
	}
	definitions := domain.Definitions()
	preferences := make([]Preference, 0, len(definitions))
	for _, definition := range definitions {
		preference := Preference{Definition: definition}
		if customization, found := byOperation[definition.Operation]; found {
			preference.Customization = &customization
			preference.Outdated = customization.Mode == domain.Rewrite && customization.BaseTemplateID != definition.TemplateID
		}
		preferences = append(preferences, preference)
	}
	return preferences, nil
}

// Save retains a guarded personal layer; it does not run or authorize generation.
func (p *Preferences) Save(ctx context.Context, actor identityapp.Principal, input SaveInput) (domain.Customization, error) {
	if p == nil || p.store == nil || p.now == nil {
		return domain.Customization{}, domain.ErrInvalidCustomization
	}
	if !canCustomize(actor) {
		return domain.Customization{}, identityapp.ErrForbidden
	}
	input.Content = strings.TrimSpace(input.Content)
	definition, found := domain.DefinitionFor(input.Operation)
	requestID, requestErr := uuid.Parse(input.RequestID)
	if !found || input.ExpectedRevision < 0 || input.ExpectedRevision >= math.MaxInt32 || input.IdempotencyKey == uuid.Nil ||
		requestErr != nil || requestID == uuid.Nil || requestID.String() != input.RequestID ||
		domain.ValidateContent(input.Operation, input.Mode, input.Content) != nil {
		return domain.Customization{}, domain.ErrInvalidCustomization
	}
	if input.BaseTemplateID != definition.TemplateID {
		return domain.Customization{}, ErrBaselineConflict
	}
	occurredAt := p.now().UTC()
	customization := domain.Customization{
		ID:        uuid.NewSHA1(uuid.NameSpaceURL, []byte(actor.OrgID.String()+"/"+actor.ID.String()+"/prompt/"+input.Operation)),
		Operation: input.Operation, Mode: input.Mode, Content: input.Content, BaseTemplateID: input.BaseTemplateID,
		Revision: input.ExpectedRevision + 1, UpdateTime: occurredAt,
	}
	if customization.Validate() != nil {
		return domain.Customization{}, domain.ErrInvalidCustomization
	}
	eventID := uuid.New()
	hash := sha256.Sum256([]byte(input.Content))
	payload, err := json.Marshal(map[string]any{
		"event_id": eventID, "event_type": "lanverse.audit.recorded.v1", "occurred_at": occurredAt, "org_id": actor.OrgID,
		"actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": "audit", "id": eventID},
		"data": map[string]any{"action": "prompt.customization_saved", "object": map[string]any{"type": "prompt_customization", "id": customization.ID},
			"request_id": input.RequestID, "before": map[string]any{"revision": input.ExpectedRevision},
			"after": map[string]any{"operation": input.Operation, "mode": input.Mode, "revision": customization.Revision, "base_template_id": input.BaseTemplateID, "content_sha256": hex.EncodeToString(hash[:])}},
	})
	if err != nil {
		return domain.Customization{}, fmt.Errorf("encode prompt preference audit: %w", err)
	}
	saved, err := p.store.SaveCustomization(ctx, actor, input, customization, identityapp.OutboxEvent{ID: eventID, Topic: "lanverse.audit.recorded.v1", PartitionKey: actor.OrgID.String(), Payload: payload})
	if err != nil {
		return domain.Customization{}, fmt.Errorf("save workspace prompt preference: %w", err)
	}
	if saved.Validate() != nil || saved.ID != customization.ID || saved.Operation != input.Operation || saved.Mode != input.Mode ||
		saved.Content != input.Content || saved.BaseTemplateID != input.BaseTemplateID || saved.Revision != input.ExpectedRevision+1 {
		return domain.Customization{}, domain.ErrInvalidCustomization
	}
	return saved, nil
}

func canCustomize(actor identityapp.Principal) bool {
	return actor.ID != uuid.Nil && actor.OrgID != uuid.Nil && !actor.MustChangePassword &&
		(actor.Role == identitydomain.RoleAdmin || actor.Role == identitydomain.RoleProducer)
}
