package catalog_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/pricesync"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/pricevalidation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

func TestParseOpenRouterPriceCandidateNormalizesSupportedTokenPricing(t *testing.T) {
	fetchedAt := time.Date(2026, 11, 8, 4, 0, 0, 0, time.UTC)
	candidate, err := pricesync.ParseOpenRouterPriceCandidate(
		"openai/gpt-test",
		json.RawMessage(`{"prompt":"0.0000025","completion":"0.00001","input_cache_read":"0.0000005"}`),
		fetchedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.AdapterKey != "openrouter" || candidate.ProviderModelID != "openai/gpt-test" ||
		candidate.Currency != "USD" || candidate.Unit == nil || *candidate.Unit != domain.PricePer1KTokens ||
		candidate.Status != domain.PriceCandidatePendingReview || !candidate.SourceFetchedAt.Equal(fetchedAt) ||
		candidate.SourceURL != pricesync.OpenRouterCatalogURL || candidate.ID == uuid.Nil || len(candidate.SourceHash) != 64 {
		t.Fatalf("candidate metadata mismatch: %+v", candidate)
	}
	var rule struct {
		Input       json.Number `json:"input_micros_per_1k"`
		CachedInput json.Number `json:"cached_input_micros_per_1k"`
		Output      json.Number `json:"output_micros_per_1k"`
	}
	if err := json.Unmarshal(candidate.Rule, &rule); err != nil {
		t.Fatal(err)
	}
	if rule.Input != "2500" || rule.CachedInput != "500" || rule.Output != "10000" {
		t.Fatalf("normalized rates: input=%s cached=%s output=%s", rule.Input, rule.CachedInput, rule.Output)
	}
	validator, err := pricevalidation.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	price := validPriceVersion(uuid.New())
	price.Unit = *candidate.Unit
	price.Rule = candidate.Rule
	price.Currency = candidate.Currency
	price.FXRateToCNY = "7.200000"
	if err := validator.Validate(price); err != nil {
		t.Fatalf("candidate rule does not satisfy the existing published-price contract: %v", err)
	}
}

func TestParseOpenRouterPriceCandidateDoesNotExecuteUnsupportedDimensions(t *testing.T) {
	for _, pricing := range []string{
		`{"prompt":"0.0000025","completion":"0.00001","overrides":[{"min_prompt_tokens":200000,"prompt":"0.000005"}]}`,
		`{"prompt":"0.0000025","completion":"0.00001","input_cache_write":"0.000003"}`,
		`{"prompt":"0.0000025","completion":"0.00001","image":"0.0001"}`,
		`{"prompt":"0.0000001","completion":"0.00001","input_cache_read":"0.0000002"}`,
	} {
		candidate, err := pricesync.ParseOpenRouterPriceCandidate("provider/model", json.RawMessage(pricing), time.Now().UTC())
		if err != nil {
			t.Fatalf("parse candidate %s: %v", pricing, err)
		}
		if candidate.Status != domain.PriceCandidateNeedsReview || candidate.Unit != nil || len(candidate.Rule) != 0 {
			t.Fatalf("unsupported pricing became executable: %+v", candidate)
		}
	}
}

func TestParseOpenRouterPriceCandidateRejectsMalformedSourceValues(t *testing.T) {
	for _, tc := range []struct {
		modelID string
		pricing string
	}{
		{"", `{"prompt":"0.0001","completion":"0.0002"}`},
		{"provider/model", `[]`},
		{"provider/model", `{"prompt":0.0001,"completion":"0.0002"}`},
		{"provider/model", `{"prompt":"-0.0001","completion":"0.0002"}`},
		{"provider/model", `{"prompt":"not-a-rate","completion":"0.0002"}`},
		{"provider/model", `{"prompt":"0.0001","completion":"0.0002"} trailing`},
		{"provider/model", `{"prompt":"0.0001","prompt":"0.0003","completion":"0.0002"}`},
	} {
		if _, err := pricesync.ParseOpenRouterPriceCandidate(tc.modelID, json.RawMessage(tc.pricing), time.Now().UTC()); err == nil {
			t.Fatalf("accepted malformed pricing: model=%q pricing=%s", tc.modelID, tc.pricing)
		}
	}
}
