package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ErrInvalidAdminModels means the requested management snapshot is invalid.
var ErrInvalidAdminModels = errors.New("invalid administrator model query")

// AdminModelVersion is immutable configuration safe to edit as a new version.
type AdminModelVersion struct {
	ID               uuid.UUID       `json:"id"`
	VersionNo        int             `json:"version_no"`
	ProviderModelID  string          `json:"provider_model_id"`
	Modes            []string        `json:"modes"`
	Limits           json.RawMessage `json:"limits" swaggertype:"object"`
	ParamSchema      json.RawMessage `json:"param_schema" swaggertype:"array,object"`
	SupportsQuery    bool            `json:"supports_query"`
	SupportsCancel   bool            `json:"supports_cancel"`
	SupportsCallback bool            `json:"supports_callback"`
	ExpectedMaxMS    int             `json:"expected_max_ms"`
	Moderation       string          `json:"moderation"`
	Queue            string          `json:"queue"`
	CreateTime       time.Time       `json:"create_time"`
}

// AdminPrice contains the published price history, including scheduled prices.
type AdminPrice struct {
	SourceURL     *string         `json:"source_url" extensions:"x-nullable"`
	ReviewedAt    *time.Time      `json:"reviewed_at" extensions:"x-nullable"`
	ID            uuid.UUID       `json:"id"`
	VersionNo     int             `json:"version_no"`
	Unit          string          `json:"unit"`
	Rule          json.RawMessage `json:"rule" swaggertype:"object"`
	Currency      string          `json:"currency"`
	FXRateToCNY   *string         `json:"fx_rate_to_cny" extensions:"x-nullable"`
	EffectiveFrom time.Time       `json:"effective_from"`
	CreateTime    time.Time       `json:"create_time"`
}

// ModelPricePreset describes the supplier's public reference and an optional supported default.
type ModelPricePreset struct {
	Currency       string          `json:"currency"`
	SourceURL      string          `json:"source_url"`
	ReferenceLines []string        `json:"reference_lines"`
	Unit           *string         `json:"unit,omitempty"`
	Rule           json.RawMessage `json:"rule,omitempty" swaggertype:"object"`
}

// AdminModelDetail combines the current identity and its real version history.
type AdminModelDetail struct {
	PricePreset *ModelPricePreset               `json:"price_preset,omitempty"`
	Candidate   *domain.ModelCandidateSelection `json:"candidate,omitempty"`
	Model       CreatedModel                    `json:"model"`
	Versions    []AdminModelVersion             `json:"versions"`
	Prices      []AdminPrice                    `json:"prices"`
}

// CapabilitySummary declares the modes and ordered reference roles for forms.
type CapabilitySummary struct {
	ID         uuid.UUID `json:"id"`
	Key        string    `json:"key"`
	OutputType string    `json:"output_type"`
	Modes      []string  `json:"modes"`
	InputRoles []string  `json:"input_roles"`
}

// AdminModelBilling describes price readiness for an installed preset.
type AdminModelBilling struct {
	Unit             string `json:"unit,omitempty"`
	ProcurementReady bool   `json:"procurement_ready"`
	SaleReady        bool   `json:"sale_ready"`
	SalePublished    bool   `json:"sale_published"`
	Status           string `json:"status"`
}

// AdminModelListInput selects a bounded stable model page.
type AdminModelListInput struct {
	Limit         int
	ProviderID    uuid.UUID
	PricingReview bool
	BillingStatus string
	After         *ModelCatalogCursor
}

// AdminModelPage returns model identities and one internal keyset position.
type AdminModelPage struct {
	Items []CreatedModel
	Next  *ModelCatalogCursor
}

// AdminModelsStore checks live administrator rights before each real table read.
type AdminModelsStore interface {
	ListAdminModels(context.Context, identityapp.Principal, AdminModelListInput) (AdminModelPage, error)
	ReadAdminModel(context.Context, identityapp.Principal, uuid.UUID) (AdminModelDetail, error)
	ListAdminCapabilities(context.Context, identityapp.Principal) ([]CapabilitySummary, error)
}

// AdminModelsQuery supplies the complete settings forms without stored secrets.
type AdminModelsQuery struct{ store AdminModelsStore }

// NewAdminModelsQuery injects the authorized catalog store.
func NewAdminModelsQuery(store AdminModelsStore) *AdminModelsQuery {
	return &AdminModelsQuery{store: store}
}

func requireAdmin(actor identityapp.Principal) error {
	if !IsPlatformAdmin(actor) || actor.OrgID == uuid.Nil {
		return identityapp.ErrForbidden
	}
	return nil
}

// List includes disabled and unpublished models for administrator settings.
func (q *AdminModelsQuery) List(ctx context.Context, actor identityapp.Principal, input AdminModelListInput) (AdminModelPage, error) {
	if err := requireAdmin(actor); err != nil {
		return AdminModelPage{}, err
	}
	if q == nil || q.store == nil ||
		(input.BillingStatus != "" && (!input.PricingReview || !slices.Contains([]string{"billable", "price_pending", "usage_pending", "inactive"}, input.BillingStatus))) ||
		input.Limit < 0 || input.Limit > 200 || (input.After != nil && (input.After.ID == uuid.Nil || input.After.Key == "" || len(input.After.Key) > 128)) {
		return AdminModelPage{}, ErrInvalidAdminModels
	}
	if input.Limit == 0 {
		input.Limit = 50
	}
	result, err := q.store.ListAdminModels(ctx, actor, input)
	if err != nil {
		return AdminModelPage{}, fmt.Errorf("list admin models: %w", err)
	}
	return result, nil
}

// Detail reads model configuration and price history in one authorized snapshot.
func (q *AdminModelsQuery) Detail(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (AdminModelDetail, error) {
	if err := requireAdmin(actor); err != nil {
		return AdminModelDetail{}, err
	}
	if q == nil || q.store == nil || id == uuid.Nil {
		return AdminModelDetail{}, ErrInvalidAdminModels
	}
	result, err := q.store.ReadAdminModel(ctx, actor, id)
	if err != nil {
		return AdminModelDetail{}, fmt.Errorf("read admin model: %w", err)
	}
	return result, nil
}

// Capabilities returns existing capability contracts used by new model forms.
func (q *AdminModelsQuery) Capabilities(ctx context.Context, actor identityapp.Principal) ([]CapabilitySummary, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	if q == nil || q.store == nil {
		return nil, ErrInvalidAdminModels
	}
	result, err := q.store.ListAdminCapabilities(ctx, actor)
	if err != nil {
		return nil, fmt.Errorf("read admin capabilities: %w", err)
	}
	return result, nil
}
