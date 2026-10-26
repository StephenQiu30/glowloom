package application

import (
	"context"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// OnboardingModel is the immutable selection prepared for first-time activation.
type OnboardingModel struct {
	ID              uuid.UUID `json:"id"`
	VersionID       uuid.UUID `json:"version_id"`
	ReleaseID       uuid.UUID `json:"release_id"`
	DisplayName     string    `json:"display_name"`
	ProviderModelID string    `json:"provider_model_id"`
	Capability      string    `json:"capability"`
	Status          string    `json:"status"`
}

// ProviderOnboardingState restores the flow from server-owned nonsecret state.
type ProviderOnboardingState struct {
	ProviderID   uuid.UUID         `json:"provider_id"`
	Revision     int64             `json:"revision"`
	Status       string            `json:"status"`
	Models       []OnboardingModel `json:"models" gorm:"-"`
	CredentialID *uuid.UUID        `json:"credential_id,omitempty"`
	Check        *ProviderCheck    `json:"check,omitempty" gorm:"-"`
}

// ActivateOnboardingInput binds explicit activation to a verified current check.
type ActivateOnboardingInput struct {
	RequestID        uuid.UUID `json:"request_id"`
	CheckID          uuid.UUID `json:"check_id"`
	ExpectedRevision int64     `json:"expected_revision"`
}

// OnboardingStore owns live authorization and atomic selected-model activation.
type OnboardingStore interface {
	ReadProviderOnboarding(context.Context, identityapp.Principal, uuid.UUID) (ProviderOnboardingState, error)
	ActivateProviderOnboarding(context.Context, identityapp.Principal, uuid.UUID, ActivateOnboardingInput) (ProviderOnboardingState, error)
}

// ProviderOnboarding coordinates the preset connection's first activation.
type ProviderOnboarding struct{ store OnboardingStore }

// NewProviderOnboarding injects the transactional catalog adapter.
func NewProviderOnboarding(store OnboardingStore) *ProviderOnboarding {
	return &ProviderOnboarding{store: store}
}

// Read resumes a saved connection without accepting secrets from the browser.
func (s *ProviderOnboarding) Read(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (ProviderOnboardingState, error) {
	if !IsPlatformAdmin(actor) {
		return ProviderOnboardingState{}, identityapp.ErrForbidden
	}
	if id == uuid.Nil {
		return ProviderOnboardingState{}, ErrInvalidProviderDetail
	}
	return s.store.ReadProviderOnboarding(ctx, actor, id)
}

// Activate enables only the original selection under one successful model check.
func (s *ProviderOnboarding) Activate(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input ActivateOnboardingInput) (ProviderOnboardingState, error) {
	if !IsPlatformAdmin(actor) {
		return ProviderOnboardingState{}, identityapp.ErrForbidden
	}
	if id == uuid.Nil || input.RequestID == uuid.Nil || input.CheckID == uuid.Nil || input.ExpectedRevision < 1 {
		return ProviderOnboardingState{}, ErrInvalidProviderDetail
	}
	return s.store.ActivateProviderOnboarding(ctx, actor, id, input)
}
