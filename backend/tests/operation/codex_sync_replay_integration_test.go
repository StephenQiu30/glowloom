package operation_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestCodexOriginalDispatchIdentityRemainsBound(t *testing.T) {
	s := newStateStore()
	i, found, err := s.LoadProviderImageIdentity(t.Context(), s.loaded.Operation.ID)
	require.NoError(t, err)
	require.True(t, found)
	require.NoError(t, i.Validate())
	changed := i
	changed.RequestKey = "another-request"
	require.NotEqual(t, i, changed)
	require.Equal(t, "submit", i.Action)
	require.EqualValues(t, 1, i.Attempt)
	require.Equal(t, application.ProviderCallStateUnset, s.evidence.SubmitState)
}
