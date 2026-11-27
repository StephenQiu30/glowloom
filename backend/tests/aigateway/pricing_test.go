package aigateway_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
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

func TestSeededTextTariffFreezesRateAndMatchesUpstream(t *testing.T) {
	_, store, model, actor := fixture(t)
	ctx := t.Context()
	conf, err := store.Configuration(ctx)
	require.NoError(t, err)
	var selected struct {
		Key        string             `json:"key"`
		BaseURL    string             `json:"base_url"`
		Upstream   string             `json:"upstream"`
		TextTariff *domain.TextTariff `json:"text_tariff"`
	}
	for _, row := range conf["model_presets"].([]json.RawMessage) {
		require.NoError(t, json.Unmarshal(row, &selected))
		if selected.Upstream == "doubao-seed-character-251128" {
			break
		}
		selected.TextTariff = nil
	}
	require.NotNil(t, selected.TextTariff)
	model.ID, model.Name, model.PresetKey = uuid.New(), uuid.NewString(), selected.Key
	model.Price = domain.Price{Currency: "CNY", FX: "1", Multiplier: "1", Discount: "1", TextTariff: selected.TextTariff}
	model.MaxInput, model.PurchaseCap = 96000, 10000000
	require.NoError(t, store.StageModel(ctx, model))
	changed := model
	changed.ID = uuid.New()
	tariff := *model.Price.TextTariff
	tariff.Tiers = slices.Clone(tariff.Tiers)
	tariff.Tiers[0].Output = "3"
	changed.Price.TextTariff = &tariff
	require.ErrorIs(t, store.StageModel(ctx, changed), domain.ErrInvalid)

	in := app.Onboarding{
		Route: domain.Route{ID: uuid.New(), ModelID: model.ID, Supplier: uuid.New(), Group: uuid.NewString(), BaseURL: selected.BaseURL,
			UpstreamModel: "wrong-model", Protocol: model.Protocol, Purchase: model.Price},
		Scope: actor.ScopeID, Concurrency: 2, RPM: 60,
	}
	require.ErrorIs(t, store.PrepareOnboarding(ctx, in), domain.ErrInvalid)
	in.Route.UpstreamModel = selected.Upstream
	require.NoError(t, store.PrepareOnboarding(ctx, in))
}
