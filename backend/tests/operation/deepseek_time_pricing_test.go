package operation_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

const deepSeekTimeRule = `{"input_micros_per_1k":2000,"cached_input_micros_per_1k":40,"output_micros_per_1k":8000,"time_pricing":"deepseek_cn_2026","markup_multiplier":"1","credit_ratio":"10"}`

func TestDeepSeekTimePricingUsesFrozenIntervalAndOneCeiling(t *testing.T) {
	price := pricingRule(catalogdomain.PricePer1KTokens, deepSeekTimeRule, "CNY", "")
	usage, err := application.ParseBillableUsage(json.RawMessage(`{"input_tokens":1000,"cached_input_tokens":800,"output_tokens":1000}`))
	require.NoError(t, err)
	// 200*2 + 800*.04 + 1000*8 = 8432 micros; off-peak = 4216.
	for _, tc := range []struct {
		name, at string
		want     int64
	}{
		{"before morning", "2026-11-04T08:59:00+08:00", 4216},
		{"morning inclusive", "2026-11-04T09:00:00+08:00", 8432},
		{"before noon", "2026-11-04T11:59:00+08:00", 8432},
		{"noon exclusive", "2026-11-04T12:00:00+08:00", 4216},
		{"afternoon inclusive", "2026-11-04T14:00:00+08:00", 8432},
		{"evening exclusive", "2026-11-04T18:00:00+08:00", 4216},
		{"UTC interpreted in Beijing", "2026-11-04T01:00:00Z", 8432},
		{"weekend", "2026-11-07T10:00:00+08:00", 4216},
		{"adjusted working Saturday", "2026-10-10T10:00:00+08:00", 4216},
		{"new year", "2026-01-02T10:00:00+08:00", 4216},
		{"spring festival", "2026-02-23T10:00:00+08:00", 4216},
		{"Qingming", "2026-04-06T10:00:00+08:00", 4216},
		{"labour day", "2026-05-05T10:00:00+08:00", 4216},
		{"dragon boat", "2026-06-19T10:00:00+08:00", 4216},
		{"mid autumn", "2026-09-25T10:00:00+08:00", 4216},
		{"national holiday", "2026-10-07T10:00:00+08:00", 4216},
		{"after national holiday", "2026-10-08T10:00:00+08:00", 8432},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, err := time.Parse(time.RFC3339, tc.at)
			require.NoError(t, err)
			got, err := application.CalculateActualCostDuring(price, usage, "text_to_text", "", start, start.Add(10*time.Second))
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
	tiny, err := application.ParseBillableUsage(json.RawMessage(`{"input_tokens":1,"cached_input_tokens":1,"output_tokens":1}`))
	require.NoError(t, err)
	at := time.Date(2026, 11, 4, 4, 0, 0, 0, time.UTC)
	got, err := application.CalculateActualCostDuring(price, tiny, "text_to_text", "", at, at.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, int64(5), got) // (0.04+8)/2, one final ceiling.
	_, err = application.CalculateActualCost(price, usage, "text_to_text", "")
	require.Error(t, err, "missing timing cannot use base peak price")
}

func TestDeepSeekTimePricingRejectsAmbiguousOrUnreviewedIntervals(t *testing.T) {
	price := pricingRule(catalogdomain.PricePer1KTokens, deepSeekTimeRule, "CNY", "")
	usage, err := application.ParseBillableUsage(json.RawMessage(`{"input_tokens":1000,"cached_input_tokens":0,"output_tokens":1000}`))
	require.NoError(t, err)
	for _, tc := range []struct{ start, end string }{
		{"2026-11-04T08:59:59+08:00", "2026-11-04T09:00:01+08:00"},
		{"2026-11-04T11:59:59+08:00", "2026-11-04T12:00:00+08:00"},
		{"2026-11-04T13:59:59+08:00", "2026-11-04T14:00:00+08:00"},
		{"2026-11-04T17:59:59+08:00", "2026-11-04T18:00:00+08:00"},
		{"2026-11-04T10:00:00+08:00", "2026-11-04T10:05:01+08:00"},
		{"2026-11-04T10:00:00+08:00", "2026-11-04T09:59:59+08:00"},
		{"2026-11-04T10:00:00+08:00", "2026-11-05T10:00:00+08:00"},
		{"2027-01-04T10:00:00+08:00", "2027-01-04T10:00:01+08:00"},
	} {
		start, err := time.Parse(time.RFC3339, tc.start)
		require.NoError(t, err)
		end, err := time.Parse(time.RFC3339, tc.end)
		require.NoError(t, err)
		_, err = application.CalculateActualCostDuring(price, usage, "text_to_text", "", start, end)
		require.Error(t, err, tc.start)
	}
	_, err = application.CalculateActualCostDuring(price, usage, "text_to_text", "", time.Time{}, time.Now())
	require.Error(t, err)
}

func TestDeepSeekQuoteReservesPeakPriceEvenOffPeak(t *testing.T) {
	price := pricingRule(catalogdomain.PricePer1KTokens, deepSeekTimeRule, "CNY", "")
	amount, detail, err := application.CalculateQuoteDetailed(price, application.PricingInput{QuotedAt: time.Date(2026, 11, 4, 4, 0, 0, 0, time.UTC), EstimatedInputTokens: 1000, MaxOutputTokens: 1000})
	require.NoError(t, err)
	require.Equal(t, int64(10000), amount)
	raw, err := json.Marshal(detail)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"time_pricing":"deepseek_cn_2026"`)
	require.Contains(t, string(raw), `"reservation_period":"peak"`)
	require.Equal(t, int64(1), *detail.EstimatedCredits)
}

func TestDeepSeekProTimePricingAndUnreviewedQuote(t *testing.T) {
	price := pricingRule(catalogdomain.PricePer1KTokens, `{"input_micros_per_1k":9000,"cached_input_micros_per_1k":300,"output_micros_per_1k":27000,"time_pricing":"deepseek_cn_2026"}`, "CNY", "")
	usage, err := application.ParseBillableUsage(json.RawMessage(`{"input_tokens":1000,"cached_input_tokens":800,"output_tokens":1000}`))
	require.NoError(t, err)
	peak := time.Date(2026, 11, 4, 1, 0, 0, 0, time.UTC)
	got, err := application.CalculateActualCostDuring(price, usage, "text_to_text", "", peak, peak.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, int64(29040), got)
	off := peak.Add(3 * time.Hour)
	got, err = application.CalculateActualCostDuring(price, usage, "text_to_text", "", off, off.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, int64(14520), got)
	_, err = application.CalculateQuote(price, application.PricingInput{QuotedAt: time.Date(2027, 1, 4, 1, 0, 0, 0, time.UTC), EstimatedInputTokens: 1000, MaxOutputTokens: 1000})
	require.Error(t, err)
}
