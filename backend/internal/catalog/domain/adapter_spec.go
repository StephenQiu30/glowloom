package domain

import (
	"slices"
	"strings"
)

// AdapterSpecification declares the protocol invariants, endpoint rules,
// allowed generation parameters, and candidate model loaders for a provider adapter.
type AdapterSpecification struct {
	DisplayName       string
	LogoKey           string
	ProtocolIDs       []string
	CheckCapabilities []string
	AdapterKey        string
	AllowedBaseURL    string
	AllowedRegion     Region
	Capabilities      []string
	IsManagedCloud    bool
	ParameterFields   map[string][]string // capability -> allowed parameter names
	CandidateCatalog  func() ModelCandidateCatalog
}

var adapterRegistry = map[string]AdapterSpecification{}

// RegisterAdapterSpec registers an adapter specification. It panics if duplicated
// to guarantee deterministic registry invariants at application startup.
func RegisterAdapterSpec(spec AdapterSpecification) {
	if spec.AdapterKey == "" {
		panic("register adapter spec with empty key")
	}
	if _, exists := adapterRegistry[spec.AdapterKey]; exists {
		panic("register duplicate adapter spec")
	}
	adapterRegistry[spec.AdapterKey] = cloneAdapterSpec(spec)
}

// GetAdapterSpec retrieves the registered specification for the given adapter key.
func GetAdapterSpec(adapter string) (AdapterSpecification, bool) {
	spec, ok := adapterRegistry[adapter]
	return cloneAdapterSpec(spec), ok
}

// AdapterSpecifications returns ordered, isolated declarations for safe projections.
func AdapterSpecifications() []AdapterSpecification {
	keys := make([]string, 0, len(adapterRegistry))
	for key := range adapterRegistry {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	specs := make([]AdapterSpecification, 0, len(keys))
	for _, key := range keys {
		specs = append(specs, cloneAdapterSpec(adapterRegistry[key]))
	}
	return specs
}

func cloneAdapterSpec(spec AdapterSpecification) AdapterSpecification {
	spec.Capabilities = slices.Clone(spec.Capabilities)
	spec.ProtocolIDs = slices.Clone(spec.ProtocolIDs)
	spec.CheckCapabilities = slices.Clone(spec.CheckCapabilities)
	fields := make(map[string][]string, len(spec.ParameterFields))
	for key, values := range spec.ParameterFields {
		fields[key] = slices.Clone(values)
	}
	spec.ParameterFields = fields
	return spec
}

// ManagedCloudAdapters returns the sorted list of all registered managed cloud adapter keys.
func ManagedCloudAdapters() []string {
	var list []string
	for k, spec := range adapterRegistry {
		if spec.IsManagedCloud {
			list = append(list, k)
		}
	}
	slices.Sort(list)
	return list
}

// ManagedCloudSQLInList returns a formatted SQL IN list (e.g. `'adapter1','adapter2'`)
// representing the single source of truth for managed cloud providers across queries.
func ManagedCloudSQLInList() string {
	adapters := ManagedCloudAdapters()
	quoted := make([]string, len(adapters))
	for i, a := range adapters {
		quoted[i] = "'" + a + "'"
	}
	return strings.Join(quoted, ",")
}

func init() {
	standardTextParams := []string{"temperature", "top_p", "max_tokens", "seed", "response_format"}

	RegisterAdapterSpec(AdapterSpecification{
		DisplayName: "火山方舟", LogoKey: "volcengine_ark", ProtocolIDs: []string{"volcengine_ark"}, CheckCapabilities: []string{"credential", "model_test"},
		AdapterKey:     "volcengine_ark",
		AllowedBaseURL: "https://ark.cn-beijing.volces.com/api/v3",
		AllowedRegion:  RegionDomestic,
		Capabilities:   []string{"text", "image", "video"},
		IsManagedCloud: true,
		ParameterFields: map[string][]string{
			"text.generate":  standardTextParams,
			"image.generate": {"size", "seed", "watermark", "quality"},
			"video.generate": {"duration", "duration_ms", "resolution", "ratio", "aspect_ratio", "generate_audio", "seed", "watermark", "camera_fixed"},
		},
		CandidateCatalog: volcengineModelCatalog,
	})

	RegisterAdapterSpec(AdapterSpecification{
		DisplayName: "OpenRouter", LogoKey: "openrouter", ProtocolIDs: []string{"openrouter"}, CheckCapabilities: []string{"credential", "model_discovery", "model_test"},
		AdapterKey:     "openrouter",
		AllowedBaseURL: "https://openrouter.ai/api/v1",
		AllowedRegion:  RegionOverseas,
		Capabilities:   []string{"text", "image", "video"},
		IsManagedCloud: true,
		ParameterFields: map[string][]string{
			"text.generate":  standardTextParams,
			"image.generate": {"resolution", "aspect_ratio", "quality", "output_format", "background", "seed", "output_compression"},
			"video.generate": {"duration", "duration_ms", "resolution", "aspect_ratio", "generate_audio", "seed"},
		},
		CandidateCatalog: func() ModelCandidateCatalog {
			catalog := ModelCandidateCatalog{Version: "2026-11-02.3", Models: []ModelCandidatePreset{}}
			add := func(key, name, id, capability, source, lifecycle string, modes ...string) {
				catalog.Models = append(catalog.Models, ModelCandidatePreset{
					Key: key, AdapterKey: "openrouter", DisplayName: name, ProviderModelID: id,
					Capability: capability, Modes: modes, SourceURL: source, ReviewedAt: "2026-11-02",
					Lifecycle: lifecycle, RequiresModelSelection: id == "",
				})
			}
			source := "https://openrouter.ai/api/v1/models"
			// ChatGPT (OpenAI) - 2026 Latest
			add("or-gpt-6-1-sol", "GPT-6.1 Sol", "openai/gpt-6.1-sol", "text.generate", source, "active", "text_to_text")
			add("or-gpt-6-1-terra", "GPT-6.1 Terra", "openai/gpt-6.1-terra", "text.generate", source, "active", "text_to_text")
			add("or-o3-pro", "o3 Pro", "openai/o3-pro", "text.generate", source, "active", "text_to_text")
			add("or-o3", "o3", "openai/o3", "text.generate", source, "active", "text_to_text")
			add("or-gpt-5-4", "GPT-5.4", "openai/gpt-5.4", "text.generate", source, "active", "text_to_text")
			add("or-gpt-4o", "GPT-4o", "openai/gpt-4o", "text.generate", source, "active", "text_to_text")

			// Anthropic (Claude) - 2026 Latest
			add("or-claude-sonnet-5-5", "Claude Sonnet 5.5", "anthropic/claude-sonnet-5.5", "text.generate", source, "active", "text_to_text")
			add("or-claude-opus-5-5", "Claude Opus 5.5", "anthropic/claude-opus-5.5", "text.generate", source, "active", "text_to_text")
			add("or-claude-haiku-5-5", "Claude Haiku 5.5", "anthropic/claude-haiku-5.5", "text.generate", source, "active", "text_to_text")
			add("or-claude-sonnet-4-6", "Claude Sonnet 4.6", "anthropic/claude-sonnet-4.6", "text.generate", source, "active", "text_to_text")
			add("or-claude-opus-4-6", "Claude Opus 4.6", "anthropic/claude-opus-4.6", "text.generate", source, "active", "text_to_text")

			// Gemini (Google) - 2026 Latest
			add("or-gemini-4-argon", "Gemini 4 Argon", "google/gemini-4-argon", "text.generate", source, "active", "text_to_text")
			add("or-gemini-3-8-flash", "Gemini 3.8 Flash", "google/gemini-3.8-flash", "text.generate", source, "active", "text_to_text")
			add("or-gemini-3-7-flash", "Gemini 3.7 Flash", "google/gemini-3.7-flash", "text.generate", source, "active", "text_to_text")
			add("or-gemini-2-5-pro", "Gemini 2.5 Pro", "google/gemini-2.5-pro", "text.generate", source, "active", "text_to_text")
			add("or-gemini-2-5-flash", "Gemini 2.5 Flash", "google/gemini-2.5-flash", "text.generate", source, "active", "text_to_text")

			// Grok (xAI) - 2026 Latest
			add("or-grok-4-7", "Grok 4.7", "x-ai/grok-4.7", "text.generate", source, "active", "text_to_text")
			add("or-grok-4-6", "Grok 4.6", "x-ai/grok-4.6", "text.generate", source, "active", "text_to_text")
			add("or-grok-4-5", "Grok 4.5", "x-ai/grok-4.5", "text.generate", source, "active", "text_to_text")
			add("or-grok-4-3", "Grok 4.3", "x-ai/grok-4.3", "text.generate", source, "active", "text_to_text")
			add("or-grok-4-20", "Grok 4.20", "x-ai/grok-4.20", "text.generate", source, "active", "text_to_text")

			// Images
			one := 1
			sourceImg := "https://openrouter.ai/api/v1/images/models"
			add("or-gpt-image", "GPT Image 2", "openai/gpt-image-2", "image.generate", sourceImg, "active", "text2image", "image2image")
			catalog.Models[len(catalog.Models)-1].MaxOutputs = &one
			add("or-gemini-image", "Nano Banana 2", "google/gemini-3.1-flash-image", "image.generate", sourceImg, "active", "text2image", "image2image")
			catalog.Models[len(catalog.Models)-1].MaxOutputs = &one
			return catalog
		},
	})

	RegisterAdapterSpec(AdapterSpecification{
		DisplayName: "BytePlus", LogoKey: "byteplus", ProtocolIDs: []string{"byteplus"}, CheckCapabilities: []string{"credential", "model_test"},
		AdapterKey:     "byteplus",
		AllowedBaseURL: "https://ark.ap-southeast.bytepluses.com/api/v3",
		AllowedRegion:  RegionOverseas,
		Capabilities:   []string{"text", "image"},
		IsManagedCloud: true,
		ParameterFields: map[string][]string{
			"text.generate":  standardTextParams,
			"image.generate": {"size", "seed", "watermark", "quality"},
		},
		CandidateCatalog: func() ModelCandidateCatalog {
			catalog := ModelCandidateCatalog{Version: "2026-10-26.1", Models: []ModelCandidatePreset{}}
			source := "https://docs.byteplus.com/en/docs/modelark/model-list"
			catalog.Models = append(catalog.Models,
				ModelCandidatePreset{Key: "bp-chat", AdapterKey: "byteplus", DisplayName: "BytePlus 文本 · 选择账号型号", Capability: "text.generate", Modes: []string{"text_to_text"}, SourceURL: source, ReviewedAt: "2026-10-20", Lifecycle: "account_selection", RequiresModelSelection: true},
				ModelCandidatePreset{Key: "bp-image", AdapterKey: "byteplus", DisplayName: "Seedream · 选择海外账号型号", Capability: "image.generate", Modes: []string{"text2image", "image2image"}, SourceURL: source, ReviewedAt: "2026-10-20", Lifecycle: "account_selection", RequiresModelSelection: true},
			)
			one := 1
			catalog.Models[1].MaxOutputs = &one
			return catalog
		},
	})

	RegisterAdapterSpec(AdapterSpecification{
		DisplayName: "DeepSeek", LogoKey: "deepseek", ProtocolIDs: []string{"deepseek"}, CheckCapabilities: []string{"credential", "model_discovery", "model_test"},
		AdapterKey:     "deepseek",
		AllowedBaseURL: "https://api.deepseek.com",
		AllowedRegion:  RegionDomestic,
		Capabilities:   []string{"text"},
		IsManagedCloud: true,
		ParameterFields: map[string][]string{
			"text.generate": standardTextParams,
		},
		CandidateCatalog: func() ModelCandidateCatalog {
			catalog := ModelCandidateCatalog{Version: "2026-10-26.1", Models: []ModelCandidatePreset{}}
			source := "https://api-docs.deepseek.com/api/list-models/"
			catalog.Models = append(catalog.Models,
				ModelCandidatePreset{Key: "ds-flash", AdapterKey: "deepseek", DisplayName: "DeepSeek Flash", ProviderModelID: "deepseek-flash", Capability: "text.generate", Modes: []string{"text_to_text"}, SourceURL: source, ReviewedAt: "2026-10-20", Lifecycle: "alias"},
				ModelCandidatePreset{Key: "ds-pro", AdapterKey: "deepseek", DisplayName: "DeepSeek V4 Pro", ProviderModelID: "deepseek-v4-pro", Capability: "text.generate", Modes: []string{"text_to_text"}, SourceURL: source, ReviewedAt: "2026-10-20", Lifecycle: "alias"},
			)
			return catalog
		},
	})

	RegisterAdapterSpec(AdapterSpecification{
		DisplayName: "OpenAI", LogoKey: "openai", ProtocolIDs: []string{"openai"}, CheckCapabilities: []string{"credential", "model_discovery", "model_test"},
		AdapterKey: "openai", AllowedBaseURL: "https://api.openai.com/v1", AllowedRegion: RegionOverseas,
		Capabilities: []string{"text", "image"}, IsManagedCloud: true,
		ParameterFields: map[string][]string{"text.generate": standardTextParams, "image.generate": {"size", "quality", "seed", "watermark"}},
		CandidateCatalog: func() ModelCandidateCatalog {
			catalog := ModelCandidateCatalog{Version: "2026-10-31.2", Models: []ModelCandidatePreset{}}
			for _, model := range []struct{ key, name, id string }{
				{"openai-sol", "GPT-6.1 Sol", "gpt-6.1-sol"},
				{"openai-astra", "GPT-6 Astra", "gpt-6-astra"},
				{"openai-terra", "GPT-6.1 Terra", "gpt-6.1-terra"},
				{"openai-luna", "GPT-6 Luna", "gpt-6-luna"},
			} {
				catalog.Models = append(catalog.Models, ModelCandidatePreset{Key: model.key, AdapterKey: "openai", DisplayName: model.name, ProviderModelID: model.id, Capability: "text.generate", Modes: []string{"text_to_text"}, SourceURL: "https://developers.openai.com/api/docs/models", ReviewedAt: "2026-10-31", Lifecycle: "active"})
			}
			one := 1
			for _, model := range []struct{ key, name, id string }{
				{"openai-image-sunburst", "GPT Image 2.5 Sunburst", "gpt-image-2.5-sunburst"},
				{"openai-image-flare", "GPT Image 2.5 Flare", "gpt-image-2.5-flare"},
			} {
				catalog.Models = append(catalog.Models, ModelCandidatePreset{Key: model.key, AdapterKey: "openai", DisplayName: model.name, ProviderModelID: model.id, Capability: "image.generate", Modes: []string{"text2image"}, MaxOutputs: &one, SourceURL: "https://developers.openai.com/api/reference/resources/images/methods/generate", ReviewedAt: "2026-10-31", Lifecycle: "active"})
			}
			return catalog
		},
	})

	RegisterAdapterSpec(AdapterSpecification{
		DisplayName: "Vercel", LogoKey: "vercel", ProtocolIDs: []string{"vercel"}, CheckCapabilities: []string{"credential", "model_discovery", "model_test"},
		AdapterKey: "vercel", AllowedBaseURL: "https://ai-gateway.vercel.sh/v1", AllowedRegion: RegionOverseas,
		Capabilities: []string{"text", "image"}, IsManagedCloud: true,
		ParameterFields: map[string][]string{"text.generate": standardTextParams, "image.generate": {"size", "quality", "seed"}},
		CandidateCatalog: func() ModelCandidateCatalog {
			catalog := ModelCandidateCatalog{Version: "2026-11-02.3", Models: []ModelCandidatePreset{}}
			source := "https://vercel.com/docs/ai/ai-gateway"
			add := func(key, name, id, capability, lifecycle string, modes ...string) {
				catalog.Models = append(catalog.Models, ModelCandidatePreset{
					Key: key, AdapterKey: "vercel", DisplayName: name, ProviderModelID: id,
					Capability: capability, Modes: modes, SourceURL: source, ReviewedAt: "2026-11-02",
					Lifecycle: lifecycle, RequiresModelSelection: id == "",
				})
			}
			// ChatGPT (OpenAI) - 2026 Latest
			add("vercel-gpt-6-1-sol", "GPT-6.1 Sol (Gateway)", "openai/gpt-6.1-sol", "text.generate", "active", "text_to_text")
			add("vercel-gpt-6-1-terra", "GPT-6.1 Terra (Gateway)", "openai/gpt-6.1-terra", "text.generate", "active", "text_to_text")
			add("vercel-o3-pro", "o3 Pro (Gateway)", "openai/o3-pro", "text.generate", "active", "text_to_text")
			add("vercel-o3", "o3 (Gateway)", "openai/o3", "text.generate", "active", "text_to_text")
			add("vercel-gpt-5-4", "GPT-5.4 (Gateway)", "openai/gpt-5.4", "text.generate", "active", "text_to_text")
			add("vercel-gpt-4o", "GPT-4o (Gateway)", "openai/gpt-4o", "text.generate", "active", "text_to_text")

			// Anthropic (Claude) - 2026 Latest
			add("vercel-claude-sonnet-5-5", "Claude Sonnet 5.5 (Gateway)", "anthropic/claude-sonnet-5.5", "text.generate", "active", "text_to_text")
			add("vercel-claude-opus-5-5", "Claude Opus 5.5 (Gateway)", "anthropic/claude-opus-5.5", "text.generate", "active", "text_to_text")
			add("vercel-claude-haiku-5-5", "Claude Haiku 5.5 (Gateway)", "anthropic/claude-haiku-5.5", "text.generate", "active", "text_to_text")
			add("vercel-claude-sonnet-4-6", "Claude Sonnet 4.6 (Gateway)", "anthropic/claude-sonnet-4.6", "text.generate", "active", "text_to_text")
			add("vercel-claude-opus-4-6", "Claude Opus 4.6 (Gateway)", "anthropic/claude-opus-4.6", "text.generate", "active", "text_to_text")

			// Gemini (Google) - 2026 Latest
			add("vercel-gemini-4-argon", "Gemini 4 Argon (Gateway)", "google/gemini-4-argon", "text.generate", "active", "text_to_text")
			add("vercel-gemini-3-8-flash", "Gemini 3.8 Flash (Gateway)", "google/gemini-3.8-flash", "text.generate", "active", "text_to_text")
			add("vercel-gemini-3-7-flash", "Gemini 3.7 Flash (Gateway)", "google/gemini-3.7-flash", "text.generate", "active", "text_to_text")
			add("vercel-gemini-2-5-pro", "Gemini 2.5 Pro (Gateway)", "google/gemini-2.5-pro", "text.generate", "active", "text_to_text")
			add("vercel-gemini-2-5-flash", "Gemini 2.5 Flash (Gateway)", "google/gemini-2.5-flash", "text.generate", "active", "text_to_text")

			// Grok (xAI / SpaceXAI) - 2026 Latest
			add("vercel-grok-4-7", "Grok 4.7 (Gateway)", "spacexai/grok-4.7", "text.generate", "active", "text_to_text")
			add("vercel-grok-4-6", "Grok 4.6 (Gateway)", "spacexai/grok-4.6", "text.generate", "active", "text_to_text")
			add("vercel-grok-4-5", "Grok 4.5 (Gateway)", "spacexai/grok-4.5", "text.generate", "active", "text_to_text")
			add("vercel-grok-4-3", "Grok 4.3 (Gateway)", "spacexai/grok-4.3", "text.generate", "active", "text_to_text")
			add("vercel-grok-4-20", "Grok 4.20 (Gateway)", "spacexai/grok-4.20-reasoning", "text.generate", "active", "text_to_text")

			// Images
			one := 1
			add("vercel-gpt-image", "GPT Image 2.5 Sunburst", "openai/gpt-image-2.5-sunburst", "image.generate", "active", "text2image")
			catalog.Models[len(catalog.Models)-1].MaxOutputs = &one
			add("vercel-gemini-image", "Nano Banana 2", "google/gemini-3.1-flash-image", "image.generate", "active", "text2image")
			catalog.Models[len(catalog.Models)-1].MaxOutputs = &one
			return catalog
		},
	})

	RegisterAdapterSpec(AdapterSpecification{
		DisplayName: "火山音乐", LogoKey: "volcengine_music", ProtocolIDs: []string{"volcengine_music"}, CheckCapabilities: []string{"model_test"},
		AdapterKey:     "volcengine_music",
		AllowedBaseURL: "https://open.volcengineapi.com",
		AllowedRegion:  RegionDomestic,
		Capabilities:   []string{"audio"},
		IsManagedCloud: true,
		ParameterFields: map[string][]string{
			"audio.generate": {"duration_ms", "music_input"},
		},
		CandidateCatalog: func() ModelCandidateCatalog {
			return creationModelCatalog("volcengine_music")
		},
	})

	RegisterAdapterSpec(AdapterSpecification{
		DisplayName: "即梦数字人", LogoKey: "volcengine_avatar", ProtocolIDs: []string{"volcengine_avatar"}, CheckCapabilities: []string{"model_test"},
		AdapterKey:     "volcengine_avatar",
		AllowedBaseURL: "https://openapi.cv.volces.com",
		AllowedRegion:  RegionDomestic,
		Capabilities:   []string{"video"},
		IsManagedCloud: true,
		ParameterFields: map[string][]string{
			"video.generate": {"output_resolution", "pe_fast_mode"},
		},
		CandidateCatalog: func() ModelCandidateCatalog {
			return creationModelCatalog("volcengine_avatar")
		},
	})

	RegisterAdapterSpec(AdapterSpecification{
		DisplayName: "即梦动作模仿", LogoKey: "volcengine_motion", ProtocolIDs: []string{"volcengine_motion"}, CheckCapabilities: []string{"model_test"},
		AdapterKey:     "volcengine_motion",
		AllowedBaseURL: "https://openapi.cv.volces.com",
		AllowedRegion:  RegionDomestic,
		Capabilities:   []string{"video"},
		IsManagedCloud: true,
		ParameterFields: map[string][]string{
			"video.generate": {"cut_result_first_second_switch"},
		},
		CandidateCatalog: func() ModelCandidateCatalog {
			return creationModelCatalog("volcengine_motion")
		},
	})

	RegisterAdapterSpec(AdapterSpecification{
		DisplayName: "豆包语音", LogoKey: "volcengine_speech", ProtocolIDs: []string{"volcengine_speech"}, CheckCapabilities: []string{"model_test"},
		AdapterKey:     "volcengine_speech",
		AllowedBaseURL: "https://openspeech.bytedance.com/api/v3",
		AllowedRegion:  RegionDomestic,
		Capabilities:   []string{"audio"},
		IsManagedCloud: true,
		ParameterFields: map[string][]string{
			"audio.generate": {"speaker", "speech_rate", "loudness_rate", "pitch_rate", "enable_subtitle"},
		},
		CandidateCatalog: doubaoSpeechCatalog,
	})

	RegisterAdapterSpec(AdapterSpecification{
		DisplayName: "Codex", LogoKey: "codex", ProtocolIDs: []string{"codex"}, CheckCapabilities: []string{},
		AdapterKey:     "codex",
		AllowedBaseURL: "",
		Capabilities:   []string{"text", "image"},
		IsManagedCloud: false,
		ParameterFields: map[string][]string{
			"text.generate":  standardTextParams,
			"image.generate": {"resolution", "seed"},
		},
		CandidateCatalog: func() ModelCandidateCatalog {
			catalog := ModelCandidateCatalog{Version: "2026-10-26.1", Models: []ModelCandidatePreset{}}
			source := "https://learn.chatgpt.com/docs/app-server"
			one := 1
			catalog.Models = append(catalog.Models,
				ModelCandidatePreset{Key: "codex-chat", AdapterKey: "codex", DisplayName: "Codex 聊天 · 选择节点模型", Capability: "text.generate", Modes: []string{"text_to_text"}, SourceURL: source, ReviewedAt: "2026-10-20", Lifecycle: "runtime_selection", RequiresModelSelection: true},
				ModelCandidatePreset{Key: "codex-image", AdapterKey: "codex", DisplayName: "Codex 生图 · 选择节点模型", Capability: "image.generate", Modes: []string{"text_to_image"}, SourceURL: source, ReviewedAt: "2026-10-20", Lifecycle: "runtime_selection", RequiresModelSelection: true, MaxOutputs: &one},
			)
			return catalog
		},
	})
}
