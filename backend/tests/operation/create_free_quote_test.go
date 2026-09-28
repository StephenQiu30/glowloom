package operation_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	catalogdomain "github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

func TestPrepareFreeQuotePricesAndFreezesValidatedPrompt(t *testing.T) {
	now := time.Now().UTC()
	input := operationapp.CreateFreeQuoteInput{
		ProjectID: uuid.New(), RequestID: uuid.NewString(),
		FreeQuoteItemInput: operationapp.FreeQuoteItemInput{
			ModelKey: "image.mock", Capability: "image.generate", Mode: "text_to_image",
			Prompt: "晚霞", Params: json.RawMessage(`{"resolution":"1080p"}`), OutputCount: 2,
		},
	}
	modelID := uuid.New()
	catalog := operationapp.FreeQuoteCatalog{
		ModelVersionID: uuid.New(), ProviderModelID: "provider-image-v1", Region: "domestic",
		Modes: []string{"text_to_image"}, Limits: json.RawMessage(`{"max_outputs":4,"resolutions":["720p","1080p"]}`),
		ParamSchema: json.RawMessage(`[{"field":"resolution","type":"string","required":true,"enum":["720p","1080p","4k"]}]`),
		Price: catalogdomain.PriceRuleVersion{
			ID: uuid.New(), ModelID: modelID, VersionNo: 1, Unit: catalogdomain.PricePerImage,
			Rule:     json.RawMessage(`{"base_micros":7,"by_resolution":{"1080p":1.5}}`),
			Currency: "CNY", EffectiveFrom: now.Add(-time.Minute),
		},
	}
	prepared, err := operationapp.PrepareFreeQuote(input, catalog, nil, now, nil)
	if err != nil || *prepared.Operation.QuoteMicros != 21 ||
		prepared.Operation.InputHash == "" || !prepared.Reusable ||
		len(prepared.Inputs) != 1 || *prepared.Inputs[0].TextValue != input.Prompt {
		t.Fatalf("prepared free quote = %+v, %v", prepared, err)
	}
	var detail struct {
		Unit            string                 `json:"unit"`
		Quantity        json.Number            `json:"quantity"`
		UnitPriceMicros int64                  `json:"unit_price_micros"`
		Outputs         int64                  `json:"outputs"`
		Multipliers     map[string]json.Number `json:"multipliers"`
		Region          string                 `json:"region"`
	}
	if err := json.Unmarshal(prepared.Operation.QuoteDetail, &detail); err != nil ||
		detail.Unit != "per_image" || detail.Quantity != "1" || detail.UnitPriceMicros != 7 ||
		detail.Outputs != 2 || detail.Multipliers["resolution_1080p"] != "1.5" ||
		detail.Region != "domestic" {
		t.Fatalf("quote cost detail = %+v, %v", detail, err)
	}
	candidate := uuid.New()
	reused, err := operationapp.PrepareFreeQuote(input, catalog, nil, now, &candidate)
	if err != nil || *reused.Operation.QuoteMicros != 0 ||
		reused.Operation.ReusedFromID == nil || *reused.Operation.ReusedFromID != candidate ||
		reused.Operation.InputHash != prepared.Operation.InputHash {
		t.Fatalf("reused free quote = %+v, %v", reused, err)
	}
	input.Params = json.RawMessage(`{"resolution":"4k"}`)
	if _, err := operationapp.PrepareFreeQuote(input, catalog, nil, now, nil); !errors.Is(err, operationapp.ErrFreeQuoteInputNotReady) {
		t.Fatalf("unsupported resolution = %v", err)
	}
	input.Params = json.RawMessage(`{"resolution":"720p"}`)
	input.OutputCount = 5
	if _, err := operationapp.PrepareFreeQuote(input, catalog, nil, now, nil); !errors.Is(err, operationapp.ErrFreeQuoteInputNotReady) {
		t.Fatalf("output over limit = %v", err)
	}
}

func TestPrepareFreeQuoteRejectsMissingRequiredAndOutOfRangeParams(t *testing.T) {
	now := time.Now().UTC()
	input := operationapp.CreateFreeQuoteInput{
		ProjectID: uuid.New(), RequestID: uuid.NewString(),
		FreeQuoteItemInput: operationapp.FreeQuoteItemInput{
			ModelKey: "image.mock", Capability: "image.generate", Mode: "text_to_image",
			Prompt: "一棵树", Params: json.RawMessage(`{}`), OutputCount: 1,
		},
	}
	catalog := operationapp.FreeQuoteCatalog{
		ModelVersionID: uuid.New(), ProviderModelID: "provider-image-v1", Region: "domestic",
		Modes: []string{"text_to_image"}, Limits: json.RawMessage(`{"max_outputs":2}`),
		ParamSchema: json.RawMessage(`[{"field":"steps","type":"integer","required":true,"min":1,"max":10,"step":2}]`),
		Price: catalogdomain.PriceRuleVersion{
			ID: uuid.New(), ModelID: uuid.New(), VersionNo: 1, Unit: catalogdomain.PricePerImage,
			Rule: json.RawMessage(`{"base_micros":1}`), Currency: "CNY", EffectiveFrom: now,
		},
	}
	for _, params := range []string{`{}`, `{"steps":2}`, `{"steps":11}`, `{"steps":"3"}`, `{"steps":3,"unknown":1}`, `{"steps":3,"steps":5}`} {
		input.Params = json.RawMessage(params)
		if _, err := operationapp.PrepareFreeQuote(input, catalog, nil, now, nil); !errors.Is(err, operationapp.ErrFreeQuoteInputNotReady) && !errors.Is(err, operationapp.ErrInvalidFreeQuote) {
			t.Fatalf("invalid params %s = %v", params, err)
		}
	}
	input.Params = json.RawMessage(`{"steps":3}`)
	if _, err := operationapp.PrepareFreeQuote(input, catalog, nil, now, nil); err != nil {
		t.Fatalf("valid stepped param: %v", err)
	}
}

func TestPrepareFreeQuotePricesExactIntegerDuration(t *testing.T) {
	now := time.Now().UTC()
	input := operationapp.CreateFreeQuoteInput{
		ProjectID: uuid.New(), RequestID: uuid.NewString(),
		FreeQuoteItemInput: operationapp.FreeQuoteItemInput{
			ModelKey: "video.mock", Capability: "video.generate", Mode: "text_to_video",
			Prompt: "海浪", Params: json.RawMessage(`{"duration_ms":1500.0}`), OutputCount: 1,
		},
	}
	catalog := operationapp.FreeQuoteCatalog{
		ModelVersionID: uuid.New(), ProviderModelID: "provider-video-v1", Region: "domestic",
		Modes: []string{"text_to_video"}, Limits: json.RawMessage(`{"max_outputs":1,"durations_ms":[1000,2000]}`),
		ParamSchema: json.RawMessage(`[{"field":"duration_ms","type":"integer","required":true,"min":1000,"max":2000}]`),
		Price: catalogdomain.PriceRuleVersion{
			ID: uuid.New(), ModelID: uuid.New(), VersionNo: 1, Unit: catalogdomain.PricePerSecond,
			Rule: json.RawMessage(`{"base_micros":1000}`), Currency: "CNY", EffectiveFrom: now,
		},
	}
	prepared, err := operationapp.PrepareFreeQuote(input, catalog, nil, now, nil)
	if err != nil || *prepared.Operation.QuoteMicros != 2000 {
		t.Fatalf("exact duration quote = %+v, %v", prepared, err)
	}
}

func TestFreeQuoteInputAllowsProjectDefaultModel(t *testing.T) {
	input := operationapp.CreateFreeQuoteInput{
		ProjectID: uuid.New(), RequestID: uuid.NewString(),
		FreeQuoteItemInput: operationapp.FreeQuoteItemInput{
			Capability: "image.generate", Mode: "text_to_image", Prompt: "海面", OutputCount: 1,
		},
	}
	if err := input.Validate(); err != nil {
		t.Fatalf("project default selection rejected: %v", err)
	}
}

func TestPrepareFreeQuoteFreezesMediaAndChecksPublishedRoles(t *testing.T) {
	now := time.Now().UTC()
	assetID := uuid.New()
	input := operationapp.CreateFreeQuoteInput{
		ProjectID: uuid.New(), RequestID: uuid.NewString(),
		FreeQuoteItemInput: operationapp.FreeQuoteItemInput{
			ModelKey: "image.mock", Capability: "image.generate", Mode: "image_to_image",
			Prompt: "海面", OutputCount: 1,
			MediaInputs: []operationapp.FreeQuoteMediaInput{{Role: "subject", MediaAssetID: assetID}},
		},
	}
	catalog := operationapp.FreeQuoteCatalog{
		ModelVersionID: uuid.New(), ProviderModelID: "provider-image-v1", Region: "domestic",
		Modes: []string{"image_to_image"}, InputRoles: []string{"prompt", "subject"},
		Limits:      json.RawMessage(`{"max_outputs":1,"roles":{"subject":{"max_count":1,"types":["image"],"max_bytes":100},"_total_images":1}}`),
		ParamSchema: json.RawMessage(`[]`),
		Price: catalogdomain.PriceRuleVersion{
			ID: uuid.New(), ModelID: uuid.New(), VersionNo: 1, Unit: catalogdomain.PricePerImage,
			Rule: json.RawMessage(`{"base_micros":1}`), Currency: "CNY", EffectiveFrom: now,
		},
	}
	facts := []operationapp.FreeQuoteMediaFact{{ID: assetID, Kind: "image", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ByteSize: 50}}
	prepared, err := operationapp.PrepareFreeQuote(input, catalog, facts, now, nil)
	if err != nil || len(prepared.Inputs) != 2 || prepared.Inputs[1].MediaAssetID == nil ||
		*prepared.Inputs[1].MediaAssetID != assetID || prepared.Inputs[1].Role != "subject" {
		t.Fatalf("media quote = %+v, %v", prepared, err)
	}
	changed := append([]operationapp.FreeQuoteMediaFact(nil), facts...)
	changed[0].SHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	other, err := operationapp.PrepareFreeQuote(input, catalog, changed, now, nil)
	if err != nil || other.Operation.InputHash == prepared.Operation.InputHash {
		t.Fatalf("changed media hash = %+v, %v", other, err)
	}
	for _, caseSpec := range []struct {
		name   string
		change func(*operationapp.FreeQuoteCatalog, []operationapp.FreeQuoteMediaFact)
	}{
		{"over bytes", func(_ *operationapp.FreeQuoteCatalog, facts []operationapp.FreeQuoteMediaFact) {
			facts[0].ByteSize = 101
		}},
		{"wrong type", func(_ *operationapp.FreeQuoteCatalog, facts []operationapp.FreeQuoteMediaFact) {
			facts[0].Kind = "video"
		}},
		{"missing digest", func(_ *operationapp.FreeQuoteCatalog, facts []operationapp.FreeQuoteMediaFact) { facts[0].SHA256 = "" }},
		{"unsupported role", func(c *operationapp.FreeQuoteCatalog, _ []operationapp.FreeQuoteMediaFact) {
			c.InputRoles = []string{"prompt"}
		}},
		{"zero count", func(c *operationapp.FreeQuoteCatalog, _ []operationapp.FreeQuoteMediaFact) {
			c.Limits = json.RawMessage(`{"max_outputs":1,"roles":{"subject":{"max_count":0,"types":["image"]}}}`)
		}},
		{"zero total", func(c *operationapp.FreeQuoteCatalog, _ []operationapp.FreeQuoteMediaFact) {
			c.Limits = json.RawMessage(`{"max_outputs":1,"roles":{"subject":{"max_count":1,"types":["image"]},"_total_images":0}}`)
		}},
		{"video too long", func(c *operationapp.FreeQuoteCatalog, facts []operationapp.FreeQuoteMediaFact) {
			duration := int64(15001)
			facts[0].Kind, facts[0].DurationMS = "video", &duration
			c.Limits = json.RawMessage(`{"max_outputs":1,"roles":{"subject":{"max_count":1,"types":["video"],"max_ms":15000}}}`)
		}},
		{"video invalid duration", func(c *operationapp.FreeQuoteCatalog, facts []operationapp.FreeQuoteMediaFact) {
			duration := int64(-1)
			facts[0].Kind, facts[0].DurationMS = "video", &duration
			c.Limits = json.RawMessage(`{"max_outputs":1,"roles":{"subject":{"max_count":1,"types":["video"],"max_ms":15000}}}`)
		}},
		{"provider resolution unsupported", func(c *operationapp.FreeQuoteCatalog, _ []operationapp.FreeQuoteMediaFact) {
			c.Limits = json.RawMessage(`{"max_outputs":1,"roles":{"subject":{"max_count":1,"types":["image"],"min_resolution":"1024x1024"}}}`)
		}},
	} {
		t.Run(caseSpec.name, func(t *testing.T) {
			alteredCatalog := catalog
			alteredFacts := append([]operationapp.FreeQuoteMediaFact(nil), facts...)
			caseSpec.change(&alteredCatalog, alteredFacts)
			if _, err := operationapp.PrepareFreeQuote(input, alteredCatalog, alteredFacts, now, nil); !errors.Is(err, operationapp.ErrFreeQuoteInputNotReady) {
				t.Fatalf("unsupported media accepted: %v", err)
			}
		})
	}
}
