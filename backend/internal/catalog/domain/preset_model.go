package domain

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// PresetModelPrice is a reviewed platform price, never an account bill.
type PresetModelPrice struct {
	Unit        PriceUnit       `json:"unit"`
	Rule        json.RawMessage `json:"rule" swaggertype:"object"`
	Currency    string          `json:"currency"`
	FXRateToCNY string          `json:"fx_rate_to_cny"`
}

// PresetModelCost carries the reviewed supplier bound required by a draft release.
type PresetModelCost struct {
	Currency   string    `json:"currency"`
	MaxMicros  int64     `json:"max_micros"`
	Source     string    `json:"source"`
	ReviewedAt time.Time `json:"reviewed_at"`
}

// PresetModelTemplate contains portable values; it cannot carry credentials,
// validation evidence, account identities or a publication state.
type PresetModelTemplate struct {
	Key             string           `json:"key"`
	Name            string           `json:"name"`
	Capability      string           `json:"capability"`
	ProviderModelID string           `json:"provider_model_id"`
	Modes           []string         `json:"modes"`
	Limits          json.RawMessage  `json:"limits" swaggertype:"object"`
	ParamSchema     json.RawMessage  `json:"param_schema" swaggertype:"array,object"`
	ExpectedMaxMS   int              `json:"expected_max_ms"`
	Price           PresetModelPrice `json:"price"`
	UpstreamCost    PresetModelCost  `json:"upstream_cost"`
}

// Version assigns fresh model identities and fixed, implemented execution behavior.
func (t PresetModelTemplate) Version(modelID, actorID uuid.UUID) ModelVersion {
	return ModelVersion{ID: uuid.New(), ModelID: modelID, VersionNo: 1, ProviderModelID: t.ProviderModelID, Modes: slices.Clone(t.Modes), Limits: slices.Clone(t.Limits), ParamSchema: slices.Clone(t.ParamSchema), SupportsQuery: t.Capability == "video.generate" || slices.Contains(t.Modes, "music_song") || slices.Contains(t.Modes, "music_instrumental"), ExpectedMaxMS: t.ExpectedMaxMS, Moderation: ModerationProvider, Queue: "provider", CreateBy: actorID}
}

// PriceVersion binds the copied rule to a new model and creation time.
func (t PresetModelTemplate) PriceVersion(modelID, actorID uuid.UUID, now time.Time) PriceRuleVersion {
	return PriceRuleVersion{ID: uuid.New(), ModelID: modelID, VersionNo: 1, Unit: t.Price.Unit, Rule: slices.Clone(t.Price.Rule), Currency: t.Price.Currency, FXRateToCNY: t.Price.FXRateToCNY, EffectiveFrom: now, CreateBy: actorID}
}

func (t PresetModelTemplate) validate(config ProviderConfig) error {
	if len(t.Key) < 1 || len(t.Key) > 32 || t.Key[0] < 'a' || t.Key[0] > 'z' || strings.TrimSpace(t.Name) == "" || len(t.Name) > 200 || len(t.ProviderModelID) > 256 {
		return ErrInvalidPreset
	}
	for _, c := range t.Key {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' {
			return ErrInvalidPreset
		}
	}
	capability := strings.TrimSuffix(t.Capability, ".generate")
	if t.Capability != capability+".generate" || !slices.Contains(config.Capabilities, capability) {
		return ErrInvalidPreset
	}
	allowedModes := map[string][]string{"text.generate": {"text_to_text"}, "image.generate": {"text2image", "image2image"}, "video.generate": {"text2video", "image2video", "omni_reference", "avatar", "motion"}, "audio.generate": {"omni_audio", "text2audio", "music_song", "music_instrumental"}}[t.Capability]
	units := map[string][]PriceUnit{"text.generate": {PricePerRequest, PricePer1KTokens}, "image.generate": {PricePerRequest, PricePerImage}, "video.generate": {PricePerRequest, PricePerSecond}, "audio.generate": {PricePerRequest, PricePer1KChars, PricePerSecond}}[t.Capability]
	if !slices.Contains(units, t.Price.Unit) {
		return ErrInvalidPreset
	}
	for _, mode := range t.Modes {
		if !slices.Contains(allowedModes, mode) {
			return ErrInvalidPreset
		}
	}
	var limits struct {
		MaxOutputs       int    `json:"max_outputs"`
		ProviderEndpoint string `json:"provider_endpoint"`
	}
	if json.Unmarshal(t.Limits, &limits) != nil || (t.Capability != "image.generate" && limits.MaxOutputs != 1) || (t.Capability == "video.generate" && limits.ProviderEndpoint != "") {
		return ErrInvalidPreset
	}
	cost := t.UpstreamCost
	if cost.MaxMicros < 0 || cost.MaxMicros > 1_000_000_000 || strings.TrimSpace(cost.Source) == "" || len(cost.Source) > 500 || cost.ReviewedAt.IsZero() || (cost.Currency != "CNY" && cost.Currency != "USD") || (t.Price.Currency != "CNY" && t.Price.Currency != "USD") {
		return ErrInvalidPreset
	}
	if t.Version(uuid.NameSpaceOID, uuid.NameSpaceOID).Validate() != nil || t.PriceVersion(uuid.NameSpaceOID, uuid.NameSpaceOID, cost.ReviewedAt).Validate() != nil {
		return ErrInvalidPreset
	}
	return nil
}
