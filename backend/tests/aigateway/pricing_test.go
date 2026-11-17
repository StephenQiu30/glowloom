package aigateway_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
)

func TestPriceAggregatesBeforeRoundingAndRejectsInvalidUsage(t *testing.T) {
	price := domain.Price{Input: 4000000, CacheRead: 1000000, Output: 12000000, Currency: "CNY", FX: "1", Multiplier: "1.5", Discount: "0.8"}
	cost, err := price.Cost(domain.Usage{Input: 300000, CacheRead: 200000, Output: 50000})
	require.NoError(t, err)
	require.EqualValues(t, 1440000, cost)
	credits, err := domain.Credits(cost)
	require.NoError(t, err)
	require.EqualValues(t, 15, credits)
	_, err = price.Cost(domain.Usage{Input: 1, CacheRead: 2})
	require.Error(t, err)
	_, err = price.Cost(domain.Usage{Input: -1})
	require.Error(t, err)
	price.FX = "NaN"
	_, err = price.Cost(domain.Usage{Input: 1})
	require.Error(t, err)
}

func TestPriceCacheWritesAndReasoningAreNotDoubleCounted(t *testing.T) {
	price := domain.Price{Input: 1000000, CacheRead: 100000, CacheWrite: 2000000, Output: 3000000, Currency: "CNY", FX: "1", Multiplier: "1", Discount: "1"}
	cost, err := price.Cost(domain.Usage{Input: 1000, CacheRead: 200, CacheWrite: 100, Output: 500})
	require.NoError(t, err)
	require.EqualValues(t, 2420, cost)
}
