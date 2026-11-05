package operation_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestOperationTaskNormalizesProviderFailureCode(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{"invalid_provider_request", "provider_invalid_parameters"}, {"provider_dispatch_cancelled", "provider_not_submitted"}, {"", "provider_not_submitted"}} {
		require.Equal(t, tc.want, application.SafeProviderFailureCode(tc.input, "not_submitted"))
	}
}
