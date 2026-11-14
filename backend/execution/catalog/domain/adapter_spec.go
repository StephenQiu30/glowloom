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
		DisplayName: "火山方舟", LogoKey: "volcengine_ark", ProtocolIDs: []string{"volcengine_ark"}, CheckCapabilities: []string{"credential", "model_discovery", "model_test"},
		AdapterKey:     "volcengine_ark",
		AllowedBaseURL: "https://ark.cn-beijing.volces.com/api/v3",
		AllowedRegion:  RegionDomestic,
		Capabilities:   []string{"text", "image", "video"},
		IsManagedCloud: true,
		ParameterFields: map[string][]string{
			"text.generate":  append(slices.Clone(standardTextParams), "thinking"),
			"image.generate": {"size", "seed", "watermark", "quality"},
			"video.generate": {"duration", "duration_ms", "resolution", "ratio", "aspect_ratio", "generate_audio", "seed", "watermark", "camera_fixed"},
		},
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
	})

	RegisterAdapterSpec(AdapterSpecification{
		DisplayName: "OpenAI", LogoKey: "openai", ProtocolIDs: []string{"openai"}, CheckCapabilities: []string{"credential", "model_discovery", "model_test"},
		AdapterKey: "openai", AllowedBaseURL: "https://api.openai.com/v1", AllowedRegion: RegionOverseas,
		Capabilities: []string{"text", "image"}, IsManagedCloud: true,
		ParameterFields: map[string][]string{"text.generate": standardTextParams, "image.generate": {"size", "quality", "seed", "watermark"}},
	})

	RegisterAdapterSpec(AdapterSpecification{
		DisplayName: "Vercel", LogoKey: "vercel", ProtocolIDs: []string{"vercel"}, CheckCapabilities: []string{"credential", "model_discovery", "model_test"},
		AdapterKey: "vercel", AllowedBaseURL: "https://ai-gateway.vercel.sh/v1", AllowedRegion: RegionOverseas,
		Capabilities: []string{"text", "image"}, IsManagedCloud: true,
		ParameterFields: map[string][]string{"text.generate": standardTextParams, "image.generate": {"size", "quality", "seed"}},
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
	})
}
