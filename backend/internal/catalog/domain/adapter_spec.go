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
			catalog := ModelCandidateCatalog{Version: "2026-10-26.1", Models: []ModelCandidatePreset{}}
			add := func(key, name, id, capability, source, lifecycle string, modes ...string) {
				catalog.Models = append(catalog.Models, ModelCandidatePreset{
					Key: key, AdapterKey: "openrouter", DisplayName: name, ProviderModelID: id,
					Capability: capability, Modes: modes, SourceURL: source, ReviewedAt: "2026-10-20",
					Lifecycle: lifecycle, RequiresModelSelection: id == "",
				})
			}
			source := "https://openrouter.ai/api/v1/models"
			add("or-gpt-chat", "GPT-5.4", "openai/gpt-5.4", "text.generate", source, "active", "text_to_text")
			add("or-claude-chat", "Claude Sonnet 4.6", "anthropic/claude-sonnet-4.6", "text.generate", source, "active", "text_to_text")
			add("or-gemini-chat", "Gemini 3.1 Pro Preview", "google/gemini-3.1-pro-preview", "text.generate", source, "preview", "text_to_text")
			source = "https://openrouter.ai/api/v1/images/models"
			add("or-gpt-image", "GPT Image 2", "openai/gpt-image-2", "image.generate", source, "active", "text2image", "image2image")
			add("or-gemini-image", "Nano Banana 2", "google/gemini-3.1-flash-image", "image.generate", source, "active", "text2image", "image2image")
			one := 1
			catalog.Models[len(catalog.Models)-1].MaxOutputs = &one
			add("or-seedream", "Seedream 4.5", "bytedance-seed/seedream-4.5", "image.generate", source, "active", "text2image", "image2image")
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
		AdapterKey:     "openai",
		AllowedBaseURL: "https://api.openai.com/v1",
		AllowedRegion:  RegionOverseas,
		Capabilities:   []string{"text", "image"},
		IsManagedCloud: true,
		ParameterFields: map[string][]string{
			"text.generate":  standardTextParams,
			"image.generate": {"size", "seed", "watermark", "quality"},
		},
		CandidateCatalog: func() ModelCandidateCatalog {
			catalog := ModelCandidateCatalog{Version: "2026-10-26.1", Models: []ModelCandidatePreset{}}
			source := "https://platform.openai.com/docs/models"
			one := 1
			catalog.Models = append(catalog.Models,
				ModelCandidatePreset{Key: "openai-gpt-4o", AdapterKey: "openai", DisplayName: "GPT-4o", ProviderModelID: "gpt-4o", Capability: "text.generate", Modes: []string{"text_to_text"}, SourceURL: source, ReviewedAt: "2026-10-31", Lifecycle: "active"},
				ModelCandidatePreset{Key: "openai-gpt-4o-mini", AdapterKey: "openai", DisplayName: "GPT-4o Mini", ProviderModelID: "gpt-4o-mini", Capability: "text.generate", Modes: []string{"text_to_text"}, SourceURL: source, ReviewedAt: "2026-10-31", Lifecycle: "active"},
				ModelCandidatePreset{Key: "openai-o1", AdapterKey: "openai", DisplayName: "OpenAI o1", ProviderModelID: "o1", Capability: "text.generate", Modes: []string{"text_to_text"}, SourceURL: source, ReviewedAt: "2026-10-31", Lifecycle: "active"},
				ModelCandidatePreset{Key: "openai-o1-mini", AdapterKey: "openai", DisplayName: "OpenAI o1-mini", ProviderModelID: "o1-mini", Capability: "text.generate", Modes: []string{"text_to_text"}, SourceURL: source, ReviewedAt: "2026-10-31", Lifecycle: "active"},
				ModelCandidatePreset{Key: "openai-o3-mini", AdapterKey: "openai", DisplayName: "OpenAI o3-mini", ProviderModelID: "o3-mini", Capability: "text.generate", Modes: []string{"text_to_text"}, SourceURL: source, ReviewedAt: "2026-10-31", Lifecycle: "active"},
				ModelCandidatePreset{Key: "openai-gpt-4-turbo", AdapterKey: "openai", DisplayName: "GPT-4 Turbo", ProviderModelID: "gpt-4-turbo", Capability: "text.generate", Modes: []string{"text_to_text"}, SourceURL: source, ReviewedAt: "2026-10-31", Lifecycle: "active"},
				ModelCandidatePreset{Key: "openai-dall-e-3", AdapterKey: "openai", DisplayName: "DALL·E 3", ProviderModelID: "dall-e-3", Capability: "image.generate", Modes: []string{"text2image"}, SourceURL: source, ReviewedAt: "2026-10-31", Lifecycle: "active", MaxOutputs: &one},
			)
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
