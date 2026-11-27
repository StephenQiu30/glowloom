package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTextTariffTiersUseTotalInputAndBoundAllPossibleBands(t *testing.T) {
	price := Price{
		Currency: "USD", FX: "7.25", Multiplier: "1", Discount: "1",
		TextTariff: &TextTariff{MaxInput: 200000, Tiers: []TextTier{
			{FromInput: 0, TextRates: TextRates{Input: "6", CacheRead: "1", CacheWrite: "10", Output: "2"}},
			{FromInput: 32001, TextRates: TextRates{Input: "4", CacheRead: "0.2", CacheWrite: "5", Output: "3", Reasoning: "4"}},
		}},
	}
	cost, err := price.Cost(Usage{Input: 32001, CacheRead: 32000, Output: 10, Reasoning: 5})
	require.NoError(t, err)
	require.EqualValues(t, 46683, cost)
	cost, err = price.Cost(Usage{Input: 32000, CacheRead: 32000, Output: 10, Reasoning: 5})
	require.NoError(t, err)
	require.EqualValues(t, 232145, cost)
	bound, err := price.UpperCost(32001, 10)
	require.NoError(t, err)
	require.EqualValues(t, 2320218, bound)
	_, err = price.Cost(Usage{Input: 200001})
	require.ErrorIs(t, err, ErrUnknown)
}

func TestTextTariffPeakUsesFrozenEvidenceAndSupplierCalendar(t *testing.T) {
	price := Price{Currency: "CNY", FX: "1", Multiplier: "1", Discount: "1",
		TextTariff: &TextTariff{
			Tiers:    []TextTier{{TextRates: TextRates{Input: "2", CacheRead: "0.04", CacheWrite: "2", Output: "8"}}},
			OffPeak:  &TextRates{Input: "1", CacheRead: "0.02", CacheWrite: "1", Output: "4"},
			Schedule: "ark_weekday_cn",
		}}
	usage := Usage{Input: 1000, CacheRead: 500, Output: 100}
	_, err := price.Cost(usage)
	require.ErrorIs(t, err, ErrUnknown)
	// 1 Oct 2026, 09:01 in Beijing. DeepSeek direct observes the holiday.
	usage.BillingStartedAt = time.Date(2026, 10, 1, 1, 1, 0, 0, time.UTC)
	usage.BillingReceivedAt = usage.BillingStartedAt.Add(time.Second)
	peak, err := price.Cost(usage)
	require.NoError(t, err)
	require.EqualValues(t, 1820, peak)
	price.TextTariff.Schedule = "deepseek_cn_2026"
	offPeak, err := price.Cost(usage)
	require.NoError(t, err)
	require.EqualValues(t, peak/2, offPeak)
	price.TextTariff.Schedule = "ark_weekday_cn"
	usage.BillingStartedAt = usage.BillingStartedAt.Add(-2 * time.Minute)
	_, err = price.Cost(usage)
	require.ErrorIs(t, err, ErrUnknown)
}

func TestOfficialMediaTariffCostsAndCaps(t *testing.T) {
	price := Price{Currency: "CNY", FX: "1", Multiplier: "1", Discount: "1",
		MediaTariff: &MediaTariff{Meter: SpeechCharacters, UnitPrice: "3", UnitSize: 10000, MaxQuantity: 16384},
	}
	amount, err := price.Cost(Usage{Characters: 334})
	require.NoError(t, err)
	require.EqualValues(t, 100200, amount)
	credits, err := Credits(amount)
	require.NoError(t, err)
	require.EqualValues(t, 2, credits)
	price.MediaTariff = &MediaTariff{Meter: OriginalAudioSeconds, UnitPrice: "1", UnitSize: 60, MaxQuantity: 120}
	amount, err = price.Cost(Usage{OriginalSeconds: "60.0000000000000000001"})
	require.NoError(t, err)
	require.EqualValues(t, 1000001, amount)
	_, err = price.Cost(Usage{OriginalSeconds: "120.0001"})
	require.ErrorIs(t, err, ErrUnknown)
	bound, err := price.UpperCost(0, 0)
	require.NoError(t, err)
	require.EqualValues(t, 2000000, bound)

	price.MediaTariff = &MediaTariff{Meter: ArkImagePixels, UnitPrice: "0.30", UnitSize: 1, MaxQuantity: 8, CutoffPixels: 2610000, HighPrice: "0.60", ExtraReferencePrice: "0.02"}
	amount, err = price.Cost(Usage{Pixels: 2610000, ReferenceImages: 3})
	require.NoError(t, err)
	require.EqualValues(t, 340000, amount)
	bound, err = price.ImageUpperCost(3)
	require.NoError(t, err)
	require.EqualValues(t, 640000, bound)
	_, err = price.Cost(Usage{ReferenceImages: 3})
	require.ErrorIs(t, err, ErrUnknown)
}

func TestImageTariffCountsClassifiedUsageOrProviderAmount(t *testing.T) {
	p := Price{Currency: "USD", FX: "7.25", Multiplier: "1.2", Discount: "0.9",
		MediaTariff: &MediaTariff{Meter: ImageTokens, UnitSize: 1000000, UnitPrice: "5", OutputTokenPrice: "30"}}
	cost, err := p.Cost(Usage{Input: 100, Output: 2000})
	require.NoError(t, err)
	require.EqualValues(t, 473715, cost)
	_, err = p.Cost(Usage{Input: 100})
	require.ErrorIs(t, err, ErrUnknown)
	p.MediaTariff = &MediaTariff{Meter: ReportedImageAmount, Adapter: "openrouter"}
	cost, err = p.Cost(Usage{ProviderAmount: "0.0605"})
	require.NoError(t, err)
	require.EqualValues(t, 473715, cost)
	_, err = p.Cost(Usage{ProviderAmount: "0"})
	require.ErrorIs(t, err, ErrUnknown)
}

func TestVideoSecondsTariffUsesFractionalVideoDuration(t *testing.T) {
	p := Price{Currency: "CNY", FX: "1", Multiplier: "1", Discount: "1",
		MediaTariff: &MediaTariff{Meter: OutputVideoSeconds, Adapter: "volcengine_motion", UnitPrice: "0.4", UnitSize: 1, MaxQuantity: 31}}
	cost, err := p.Cost(Usage{VideoDurationMS: 6125})
	require.NoError(t, err)
	require.EqualValues(t, 2450000, cost)
	credits, err := Credits(cost)
	require.NoError(t, err)
	require.EqualValues(t, 25, credits)
	cost, err = p.Cost(Usage{VideoDurationMS: 31001})
	require.NoError(t, err)
	require.EqualValues(t, 12400400, cost)
	_, err = p.Cost(Usage{})
	require.ErrorIs(t, err, ErrUnknown)
}
