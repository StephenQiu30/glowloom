package domain

import (
	"encoding/json"
	"math"
	"strings"

	"github.com/google/uuid"
)

// QuickOnboardingInput selects a versioned preset and an authorized verification wallet.
type QuickOnboardingInput struct {
	PresetKey string    `json:"preset_key" binding:"required"`
	ScopeID   uuid.UUID `json:"scope_id" binding:"required"`
	ProjectID uuid.UUID `json:"project_id" binding:"required"`
}

// ArkQuickOnboarding is the reviewed initial release, independent of public estimate ranges.
type ArkQuickOnboarding struct {
	Resolution           string `json:"resolution"`
	Ratio                string `json:"ratio"`
	TestPrompt           string `json:"test_prompt"`
	TestDurationSeconds  int64  `json:"test_duration_seconds"`
	SaleMicrosPerMillion int64  `json:"sale_micros_per_million"`
	Concurrency          int    `json:"concurrency"`
	RPM                  int    `json:"rpm"`
}

// SeedanceDraft compiles the fixed video release from its own sale tier and the current
// reviewed public purchase tariff. The resulting version freezes both inputs.
func (p ArkQuickOnboarding) SeedanceDraft(key, name string, profile ArkVideoEstimateProfile) (Model, Price, MediaInput, error) {
	rates := profile.Resolutions[p.Resolution]
	size, ok := rates.Geometries[p.Ratio]
	if !ok || size.Width <= 0 || size.Height <= 0 || profile.FramesPerSecond <= 0 || profile.PixelsPerToken <= 0 ||
		profile.MinOutputSeconds <= 0 || p.TestDurationSeconds < profile.MinOutputSeconds || p.TestDurationSeconds > profile.MaxOutputSeconds ||
		p.SaleMicrosPerMillion <= 0 || rates.CNYPerMillionWithoutVideo <= 0 || rates.CNYPerMillionWithoutVideo > math.MaxInt64/1_000_000 ||
		strings.TrimSpace(p.TestPrompt) == "" {
		return Model{}, Price{}, MediaInput{}, ErrInvalid
	}
	limit, err := ceil(videoFrameTokens(profile.MaxOutputSeconds, size, profile.FramesPerSecond, profile.PixelsPerToken))
	if err != nil {
		return Model{}, Price{}, MediaInput{}, err
	}
	sale := Price{Currency: "CNY", FX: "1", Multiplier: "1", Discount: "1", Output: p.SaleMicrosPerMillion}
	purchase := sale
	purchase.Output = rates.CNYPerMillionWithoutVideo * 1_000_000
	purchaseCap, err := purchase.Cost(Usage{Output: limit})
	if err != nil {
		return Model{}, Price{}, MediaInput{}, err
	}
	resolution, _ := json.Marshal(p.Resolution)
	ratio, _ := json.Marshal(p.Ratio)
	duration, _ := json.Marshal(p.TestDurationSeconds)
	minSeconds, maxSeconds := float64(profile.MinOutputSeconds), float64(profile.MaxOutputSeconds)
	m := Model{
		PresetKey: key, Name: name, Protocol: "media", Capability: "video.generate", Modes: []string{"text2video"},
		Price: sale, MaxInput: 1, MaxOutput: limit, PurchaseCap: purchaseCap, OutputPolicy: "provider_success_v1",
		Parameters: map[string]Parameter{
			"resolution": {Type: "string", Default: resolution, Enum: []json.RawMessage{resolution}},
			"ratio":      {Type: "string", Default: ratio, Enum: []json.RawMessage{ratio}},
			"duration":   {Type: "integer", Default: duration, Min: &minSeconds, Max: &maxSeconds},
		},
		VideoMetering: &VideoMetering{Resolution: p.Resolution, FramesPerSecond: profile.FramesPerSecond,
			PixelsPerToken: profile.PixelsPerToken, Geometries: map[string]VideoGeometry{p.Ratio: size}},
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return Model{}, Price{}, MediaInput{}, err
	}
	m.ID = uuid.NewSHA1(uuid.NameSpaceURL, raw)
	probe := MediaInput{Model: m.Name, Capability: m.Capability, Mode: "text2video", OutputCount: 1, Prompt: p.TestPrompt}
	return m, purchase, probe, m.Validate()
}
