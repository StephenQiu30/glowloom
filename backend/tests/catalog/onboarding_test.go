package catalog_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/paramvalidation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

func TestArkOnboardingPresetsHaveExecutablePinnedVersions(t *testing.T) {
	catalog := domain.BuiltinModelCatalog("volcengine_ark")
	validator, err := paramvalidation.NewValidator()
	require.NoError(t, err)
	capabilities := map[string]bool{}
	for _, candidate := range catalog.Models {
		require.False(t, candidate.RequiresModelSelection)
		require.Regexp(t, `-\d{6}$`, candidate.ProviderModelID)
		version, err := candidate.OnboardingVersion(uuid.New(), uuid.New())
		require.NoError(t, err)
		require.NoError(t, version.Validate())
		require.NoError(t, validator.Validate(version))
		require.Equal(t, candidate.ProviderModelID, version.ProviderModelID)
		capabilities[candidate.Capability] = true
	}
	require.Len(t, capabilities, 3)
	_, err = (domain.ModelCandidatePreset{RequiresModelSelection: true}).OnboardingVersion(uuid.New(), uuid.New())
	require.Error(t, err)
}
