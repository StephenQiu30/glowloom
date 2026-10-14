package application

import (
	"context"
	"errors"
	"time"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/google/uuid"
)

// ErrReleaseUnverified prevents publication without matching execution evidence.
var ErrReleaseUnverified = errors.New("model release lacks verified execution evidence")

// UpstreamCost records a reviewed supplier cost bound, separate from user pricing.
type UpstreamCost struct {
	Currency   string    `json:"currency"`
	MaxMicros  int64     `json:"max_micros"`
	Source     string    `json:"source"`
	ReviewedAt time.Time `json:"reviewed_at"`
}

// CreateReleaseInput pins the exact version/configuration/price to validate.
type CreateReleaseInput struct {
	ModelVersionID  uuid.UUID    `json:"model_version_id"`
	PriceVersionID  uuid.UUID    `json:"price_version_id"`
	ConfigVersionID uuid.UUID    `json:"config_version_id"`
	UpstreamCost    UpstreamCost `json:"upstream_cost" gorm:"-"`
	RequestID       uuid.UUID    `json:"request_id"`
}

// ModelRelease is an immutable binding with an explicit publication lifecycle.
type ModelRelease struct {
	ID              uuid.UUID    `json:"id"`
	ModelID         uuid.UUID    `json:"model_id"`
	ModelVersionID  uuid.UUID    `json:"model_version_id"`
	PriceVersionID  uuid.UUID    `json:"price_version_id"`
	ConfigVersionID uuid.UUID    `json:"config_version_id"`
	Status          string       `json:"status"`
	Revision        int64        `json:"revision"`
	UpstreamCost    UpstreamCost `json:"upstream_cost" gorm:"-"`
	ValidatedModes  []string     `json:"validated_modes" gorm:"-"`
	RequiredModes   []string     `json:"required_modes" gorm:"-"`
	CreatedAt       time.Time    `json:"created_at"`
}

// ActivateReleaseInput performs an explicit atomic release/default switch.
type ActivateReleaseInput struct {
	ExpectedModelRevision   int64      `json:"expected_model_revision"`
	ExpectedActiveReleaseID *uuid.UUID `json:"expected_active_release_id"`
	ExpectedRevision        int64      `json:"expected_revision"`
	EnableProvider          bool       `json:"enable_provider"`
	SetDefault              bool       `json:"set_default"`
	ExpectedDefaultRevision int64      `json:"expected_default_revision"`
	RequestID               uuid.UUID  `json:"request_id"`
}

// CapabilityDefault is a release-pinned default with its own concurrency revision.
type CapabilityDefault struct {
	Capability string    `json:"capability"`
	ReleaseID  uuid.UUID `json:"release_id"`
	Revision   int64     `json:"revision"`
}

// ReleaseStore rechecks bindings and all evidence under transaction locks.
type ReleaseStore interface {
	ListModelReleases(context.Context, identityapp.Principal, uuid.UUID) ([]ModelRelease, error)
	CreateModelRelease(context.Context, identityapp.Principal, uuid.UUID, CreateReleaseInput) (ModelRelease, error)
	ActivateModelRelease(context.Context, identityapp.Principal, uuid.UUID, ActivateReleaseInput) (ModelRelease, error)
	ListCapabilityDefaults(context.Context, identityapp.Principal) ([]CapabilityDefault, error)
}

// ModelReleases coordinates draft, validation and publication of models.
type ModelReleases struct{ store ReleaseStore }

// NewModelReleases injects release persistence.
func NewModelReleases(store ReleaseStore) *ModelReleases { return &ModelReleases{store: store} }

// List reads the saved release history for one model.
func (s *ModelReleases) List(ctx context.Context, actor identityapp.Principal, id uuid.UUID) ([]ModelRelease, error) {
	if !IsPlatformAdmin(actor) {
		return nil, identityapp.ErrForbidden
	}
	return s.store.ListModelReleases(ctx, actor, id)
}

// Create freezes the reviewed supplier cost ceiling and configuration bindings.
func (s *ModelReleases) Create(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input CreateReleaseInput) (ModelRelease, error) {
	if !IsPlatformAdmin(actor) {
		return ModelRelease{}, identityapp.ErrForbidden
	}
	cost := input.UpstreamCost
	if id == uuid.Nil || input.ModelVersionID == uuid.Nil || input.PriceVersionID == uuid.Nil || input.ConfigVersionID == uuid.Nil || input.RequestID == uuid.Nil || cost.MaxMicros < 0 || cost.MaxMicros > 1_000_000_000 || cost.Source == "" || len(cost.Source) > 500 || cost.ReviewedAt.IsZero() || (cost.Currency != "CNY" && cost.Currency != "USD") {
		return ModelRelease{}, ErrInvalidAdminModels
	}
	return s.store.CreateModelRelease(ctx, actor, id, input)
}

// Activate publishes only after every advertised mode has matching real evidence.
func (s *ModelReleases) Activate(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input ActivateReleaseInput) (ModelRelease, error) {
	if !IsPlatformAdmin(actor) {
		return ModelRelease{}, identityapp.ErrForbidden
	}
	if id == uuid.Nil || input.RequestID == uuid.Nil || input.ExpectedRevision < 1 || input.ExpectedModelRevision < 1 || input.ExpectedDefaultRevision < 0 {
		return ModelRelease{}, ErrInvalidAdminModels
	}
	return s.store.ActivateModelRelease(ctx, actor, id, input)
}

// Defaults returns explicit platform capability defaults for management.
func (s *ModelReleases) Defaults(ctx context.Context, actor identityapp.Principal) ([]CapabilityDefault, error) {
	if !IsPlatformAdmin(actor) {
		return nil, identityapp.ErrForbidden
	}
	return s.store.ListCapabilityDefaults(ctx, actor)
}
