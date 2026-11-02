package domain

import (
	"slices"
	"strings"
)

// AdapterSpecification declares the protocol invariants, endpoint rules,
// allowed generation parameters, and candidate model loaders for a provider adapter.
type AdapterSpecification struct {
	AdapterKey       string
	AllowedBaseURL   string
	AllowedRegion    Region
	Capabilities     []string
	IsManagedCloud   bool
	ParameterFields  map[string][]string // capability -> allowed parameter names
	CandidateCatalog func() ModelCandidateCatalog
}

var adapterRegistry = map[string]AdapterSpecification{}

// RegisterAdapterSpec registers an adapter specification. It panics if duplicated
// to guarantee deterministic registry invariants at application startup.
func RegisterAdapterSpec(spec AdapterSpecification) {
	if spec.AdapterKey == "" {
		panic("register adapter spec with empty key")
	}
	adapterRegistry[spec.AdapterKey] = spec
}

// GetAdapterSpec retrieves the registered specification for the given adapter key.
func GetAdapterSpec(adapter string) (AdapterSpecification, bool) {
	spec, ok := adapterRegistry[adapter]
	return spec, ok
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
			catalog := ModelCandidateCatalog{Version: "2026-11-02.1", Models: []ModelCandidatePreset{}}
			add := func(key, name, id, capability, source, lifecycle string, modes ...string) {
				catalog.Models = append(catalog.Models, ModelCandidatePreset{
					Key: key, AdapterKey: "openrouter", DisplayName: name, ProviderModelID: id,
					Capability: capability, Modes: modes, SourceURL: source, ReviewedAt: "2026-11-02",
					Lifecycle: lifecycle, RequiresModelSelection: id == "",
				})
			}
			source := "https://openrouter.ai/api/v1/models"
			// ChatGPT (OpenAI)
			add("or-gpt-4o", "GPT-4o", "openai/gpt-4o", "text.generate", source, "active", "text_to_text")
			add("or-gpt-4o-mini", "GPT-4o mini", "openai/gpt-4o-mini", "text.generate", source, "active", "text_to_text")
			add("or-o1", "o1", "openai/o1", "text.generate", source, "active", "text_to_text")
			add("or-o3-mini", "o3-mini", "openai/o3-mini", "text.generate", source, "active", "text_to_text")

			// Anthropic (Claude)
			add("or-claude-3-7", "Claude 3.7 Sonnet", "anthropic/claude-3.7-sonnet", "text.generate", source, "active", "text_to_text")
			add("or-claude-3-5", "Claude 3.5 Sonnet", "anthropic/claude-3.5-sonnet", "text.generate", source, "active", "text_to_text")
			add("or-claude-3-5-haiku", "Claude 3.5 Haiku", "anthropic/claude-3.5-haiku", "text.generate", source, "active", "text_to_text")

			// Gemini (Google)
			add("or-gemini-2-5-pro", "Gemini 2.5 Pro", "google/gemini-2.5-pro", "text.generate", source, "active", "text_to_text")
			add("or-gemini-2-5-flash", "Gemini 2.5 Flash", "google/gemini-2.5-flash", "text.generate", source, "active", "text_to_text")
			add("or-gemini-2-0-flash", "Gemini 2.0 Flash", "google/gemini-2.0-flash-001", "text.generate", source, "active", "text_to_text")

			// Grok (xAI)
			add("or-grok-2", "Grok 2", "x-ai/grok-2", "text.generate", source, "active", "text_to_text")
			add("or-grok-2-vision", "Grok 2 Vision", "x-ai/grok-2-vision", "text.generate", source, "active", "text_to_text")
			add("or-grok-beta", "Grok Beta", "x-ai/grok-beta", "text.generate", source, "active", "text_to_text")
			return catalog
		},
	})

	RegisterAdapterSpec(AdapterSpecification{
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
		AdapterKey: "vercel", AllowedBaseURL: "https://ai-gateway.vercel.sh/v1", AllowedRegion: RegionOverseas,
		Capabilities: []string{"text", "image"}, IsManagedCloud: true,
		ParameterFields: map[string][]string{"text.generate": standardTextParams, "image.generate": {"size", "quality", "seed"}},
		CandidateCatalog: func() ModelCandidateCatalog {
			catalog := ModelCandidateCatalog{Version: "2026-11-02.2", Models: []ModelCandidatePreset{}}
			source := "https://vercel.com/docs/ai/ai-gateway"
			add := func(key, name, id, capability, lifecycle string, modes ...string) {
				catalog.Models = append(catalog.Models, ModelCandidatePreset{
					Key: key, AdapterKey: "vercel", DisplayName: name, ProviderModelID: id,
					Capability: capability, Modes: modes, SourceURL: source, ReviewedAt: "2026-11-02",
					Lifecycle: lifecycle, RequiresModelSelection: id == "",
				})
			}
			// ChatGPT (OpenAI)
			add("vercel-gpt-4o", "GPT-4o (Gateway)", "openai/gpt-4o", "text.generate", "active", "text_to_text")
			add("vercel-gpt-4o-mini", "GPT-4o Mini (Gateway)", "openai/gpt-4o-mini", "text.generate", "active", "text_to_text")
			add("vercel-o1", "o1 (Gateway)", "openai/o1", "text.generate", "active", "text_to_text")
			add("vercel-o3-mini", "o3-mini (Gateway)", "openai/o3-mini", "text.generate", "active", "text_to_text")

			// Anthropic (Claude)
			add("vercel-claude-3-7", "Claude 3.7 Sonnet (Gateway)", "anthropic/claude-3-7-sonnet", "text.generate", "active", "text_to_text")
			add("vercel-claude-3-5", "Claude 3.5 Sonnet (Gateway)", "anthropic/claude-3-5-sonnet", "text.generate", "active", "text_to_text")
			add("vercel-claude-3-5-haiku", "Claude 3.5 Haiku (Gateway)", "anthropic/claude-3.5-haiku", "text.generate", "active", "text_to_text")

			// Gemini (Google)
			add("vercel-gemini-2-5-pro", "Gemini 2.5 Pro (Gateway)", "google/gemini-2.5-pro", "text.generate", "active", "text_to_text")
			add("vercel-gemini-2-5-flash", "Gemini 2.5 Flash (Gateway)", "google/gemini-2.5-flash", "text.generate", "active", "text_to_text")
			add("vercel-gemini-2-0-flash", "Gemini 2.0 Flash (Gateway)", "google/gemini-2.0-flash", "text.generate", "active", "text_to_text")

			// Grok (xAI)
			add("vercel-grok-2", "Grok 2 (Gateway)", "x-ai/grok-2", "text.generate", "active", "text_to_text")
			add("vercel-grok-2-vision", "Grok 2 Vision (Gateway)", "x-ai/grok-2-vision", "text.generate", "active", "text_to_text")
			add("vercel-grok-beta", "Grok Beta (Gateway)", "x-ai/grok-beta", "text.generate", "active", "text_to_text")
			return catalog
		},
	})

	RegisterAdapterSpec(AdapterSpecification{
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
