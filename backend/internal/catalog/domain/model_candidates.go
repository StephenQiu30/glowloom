package domain

import (
	"slices"
	"strings"
)

// ModelCandidatePreset is public configuration guidance, never account access,
// a price, a release, or evidence that inference succeeded.
type ModelCandidatePreset struct {
	Key                    string   `json:"key"`
	AdapterKey             string   `json:"adapter_key"`
	DisplayName            string   `json:"display_name"`
	ProviderModelID        string   `json:"provider_model_id"`
	Capability             string   `json:"capability"`
	Modes                  []string `json:"modes"`
	SourceURL              string   `json:"source_url"`
	ReviewedAt             string   `json:"reviewed_at"`
	Lifecycle              string   `json:"lifecycle"`
	RequiresModelSelection bool     `json:"requires_model_selection"`
	MaxOutputs             *int     `json:"max_outputs,omitempty"`
}

// ModelCandidateCatalog versions selections independently of priced v1 templates.
type ModelCandidateCatalog struct {
	Version string                 `json:"version"`
	Models  []ModelCandidatePreset `json:"models"`
}

// ModelCandidateSelection preserves the public facts chosen at connection time.
// Later catalog updates never overwrite administrator configuration.
type ModelCandidateSelection struct {
	CatalogVersion string               `json:"catalog_version"`
	Model          ModelCandidatePreset `json:"model"`
}

// BuiltinModelCatalog returns new slices on every call. Account-specific slots
// intentionally have no request model ID; they cannot be dispatched.
func BuiltinModelCatalog(adapter string) ModelCandidateCatalog {
	catalog := ModelCandidateCatalog{Version: "2026-10-21.1", Models: []ModelCandidatePreset{}}
	add := func(key, name, id, capability, source, lifecycle string, modes ...string) {
		catalog.Models = append(catalog.Models, ModelCandidatePreset{
			Key: key, AdapterKey: adapter, DisplayName: name, ProviderModelID: id,
			Capability: capability, Modes: modes, SourceURL: source, ReviewedAt: "2026-10-20",
			Lifecycle: lifecycle, RequiresModelSelection: id == "",
		})
	}
	switch adapter {
	case "deepseek":
		source := "https://api-docs.deepseek.com/api/list-models/"
		add("ds-flash", "DeepSeek Flash", "deepseek-flash", "text.generate", source, "alias", "text_to_text")
		add("ds-pro", "DeepSeek V4 Pro", "deepseek-v4-pro", "text.generate", source, "alias", "text_to_text")
	case "openrouter":
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
	case "volcengine_ark":
		source := "https://docs.volcengine.com/docs/ark/model-list"
		add("ark-chat", "豆包 · 选择账号型号", "", "text.generate", source, "account_selection", "text_to_text")
		add("ark-image", "Seedream · 选择账号型号", "", "image.generate", source, "account_selection", "text2image", "image2image")
		add("ark-video", "Seedance · 选择账号型号", "", "video.generate", source, "account_selection", "text2video", "image2video", "omni_reference")
	case "byteplus":
		source := "https://docs.byteplus.com/en/docs/modelark/model-list"
		add("bp-chat", "BytePlus 文本 · 选择账号型号", "", "text.generate", source, "account_selection", "text_to_text")
		add("bp-image", "Seedream · 选择海外账号型号", "", "image.generate", source, "account_selection", "text2image", "image2image")
	case "codex":
		source := "https://learn.chatgpt.com/docs/app-server"
		add("codex-chat", "Codex 聊天 · 选择节点模型", "", "text.generate", source, "runtime_selection", "text_to_text")
		add("codex-image", "Codex 生图 · 选择节点模型", "", "image.generate", source, "runtime_selection", "text_to_image")
		one := 1
		catalog.Models[len(catalog.Models)-1].MaxOutputs = &one
	}
	if adapter == "volcengine_ark" || adapter == "byteplus" {
		for i := range catalog.Models {
			if catalog.Models[i].Capability == "image.generate" {
				one := 1
				catalog.Models[i].MaxOutputs = &one
			}
		}
	}
	return catalog
}

// Select resolves an ordered selection against one exact catalog snapshot.
func (c ModelCandidateCatalog) Select(version string, keys []string) ([]ModelCandidatePreset, error) {
	if len(keys) > 16 || (len(keys) > 0 && version != c.Version) {
		return nil, ErrInvalidPreset
	}
	selected := make([]ModelCandidatePreset, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		if seen[key] {
			return nil, ErrInvalidPreset
		}
		seen[key] = true
		found := false
		for _, candidate := range c.Models {
			if candidate.Key == key {
				selected = append(selected, candidate)
				found = true
				break
			}
		}
		if !found {
			return nil, ErrInvalidPreset
		}
	}
	return selected, nil
}

// ModelCatalogForConfig only suggests capabilities declared by this immutable
// connection snapshot. Historical video-only presets do not acquire text access.
func ModelCatalogForConfig(config ProviderConfig) ModelCandidateCatalog {
	catalog := BuiltinModelCatalog(config.AdapterKey)
	models := make([]ModelCandidatePreset, 0, len(catalog.Models))
	for _, candidate := range catalog.Models {
		if slices.Contains(config.Capabilities, strings.TrimSuffix(candidate.Capability, ".generate")) {
			models = append(models, candidate)
		}
	}
	catalog.Models = models
	return catalog
}
