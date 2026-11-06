package billing_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/billing/domain"
)

func TestWholeCreditMicrosRoundsOneLogicalOperation(t *testing.T) {
	for _, tc := range []struct{ sale, charge int64 }{
		{0, 0}, {1, 100_000}, {100_000, 100_000}, {120_000, 200_000}, {240_000, 300_000},
	} {
		amount, err := domain.WholeCreditMicros(tc.sale)
		require.NoError(t, err)
		require.Equal(t, tc.charge, amount)
	}
	_, err := domain.WholeCreditMicros(math.MaxInt64)
	require.Error(t, err)
	_, err = domain.WholeCreditMicros(-1)
	require.Error(t, err)
}
