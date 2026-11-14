package catalog_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

type customProviderStore struct {
	created int
	input   catalog.CustomProviderInput
	logo    int
}

func (s *customProviderStore) CreateCustomProvider(_ context.Context, _ identity.Principal, input catalog.CustomProviderInput) (catalog.PresetConnection, error) {
	s.created++
	s.input = input
	return catalog.PresetConnection{}, nil
}
func (s *customProviderStore) SetProviderLogo(_ context.Context, _ identity.Principal, _ uuid.UUID, _ catalog.ProviderLogoInput) (catalog.CreatedProvider, error) {
	s.logo++
	return catalog.CreatedProvider{}, nil
}

func TestCustomProviderValidatesBeforePersistence(t *testing.T) {
	spec, ok := domain.GetAdapterSpec("openai")
	require.True(t, ok)
	input := catalog.CustomProviderInput{RequestID: uuid.New(), Name: " 网关名称 ", Config: domain.ProviderConfig{AdapterKey: "openai", BaseURL: "https://gateway.example/v1", Region: spec.AllowedRegion, Capabilities: []string{"text"}, TimeoutSeconds: 60, ConcurrencyLimit: 10, RateLimitPerMin: 60}}
	store := &customProviderStore{}
	service := catalog.NewCustomProviders(store)
	actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New(), PlatformRole: "platform_admin"}
	_, err := service.Create(t.Context(), actor, input)
	require.NoError(t, err)
	require.Equal(t, "网关名称", store.input.Name)
	for _, name := range []string{" ", strings.Repeat("名", 101), "invalid\nname", string([]byte{0xff})} {
		input.Name = name
		_, err = service.Create(t.Context(), actor, input)
		require.ErrorIs(t, err, domain.ErrInvalidPreset)
	}
	actor.PlatformRole = ""
	_, err = service.Create(t.Context(), actor, input)
	require.ErrorIs(t, err, identity.ErrForbidden)
	require.Equal(t, 1, store.created)
	_, err = service.SetLogo(t.Context(), actor, uuid.New(), catalog.ProviderLogoInput{RequestID: uuid.New(), ExpectedRevision: 1})
	require.ErrorIs(t, err, identity.ErrForbidden)
	require.Zero(t, store.logo)
}
