package domain

import (
	"encoding/json"
	"strings"
)

// volcengineModelCatalog is the official generation catalog reviewed on 2026-10-28.
// Historical keys remain stable; saved connection/version snapshots are never rewritten.
func volcengineModelCatalog() ModelCandidateCatalog {
	catalog := ModelCandidateCatalog{Version: "2026-10-28.1", Models: []ModelCandidatePreset{}}
	add := func(key, name, id, capability, lifecycle string, modes ...string) {
		model := ModelCandidatePreset{Key: key, AdapterKey: "volcengine_ark", DisplayName: name, ProviderModelID: id, Capability: capability, Modes: modes, SourceURL: "https://docs.volcengine.com/docs/ark/model-list?lang=zh", ReviewedAt: "2026-10-28", Lifecycle: lifecycle}
		if capability == "image.generate" {
			one := 1
			model.MaxOutputs = &one
		}
		catalog.Models = append(catalog.Models, model)
	}
	add("ark-chat", "Doubao Seed 2.1 Pro", "doubao-seed-2-1-pro-260915", "text.generate", "active", "text_to_text")
	add("ark-chat-lite", "Doubao Seed 2.1 Lite", "doubao-seed-2-1-lite-260915", "text.generate", "active", "text_to_text")
	add("ark-chat-turbo", "Doubao Seed 2.1 Turbo", "doubao-seed-2-1-turbo-260628", "text.generate", "active", "text_to_text")
	add("ark-seed-evolving", "Doubao Seed Evolving", "doubao-seed-evolving", "text.generate", "alias", "text_to_text")
	add("ark-chat-pro-0628", "Doubao Seed 2.1 Pro · 260628", "doubao-seed-2-1-pro-260628", "text.generate", "active", "text_to_text")
	add("ark-chat-lite-20", "Doubao Seed 2.0 Lite", "doubao-seed-2-0-lite-260428", "text.generate", "active", "text_to_text")
	add("ark-chat-mini-20", "Doubao Seed 2.0 Mini", "doubao-seed-2-0-mini-260428", "text.generate", "active", "text_to_text")
	add("ark-character", "Doubao Seed Character", "doubao-seed-character-260628", "text.generate", "active", "text_to_text")
	add("ark-character-251128", "Doubao Seed Character · 251128", "doubao-seed-character-251128", "text.generate", "active", "text_to_text")
	add("ark-translation", "Doubao Seed Translation", "doubao-seed-translation-250915", "text.generate", "active", "text_to_text")
	add("ark-kimi-k31", "Kimi K3.1 Pro", "kimi-k3-1-pro-261024", "text.generate", "active", "text_to_text")
	add("ark-deepseek-v41", "DeepSeek V4.1 Flash", "deepseek-v4-1-flash-261017", "text.generate", "active", "text_to_text")
	add("ark-deepseek-v41-0910", "DeepSeek V4.1 Flash · 260910", "deepseek-v4-1-flash-260910", "text.generate", "active", "text_to_text")
	add("ark-deepseek-economy", "DeepSeek V4.1 Flash Economy", "deepseek-v4-1-flash-economy-261017", "text.generate", "active", "text_to_text")
	add("ark-deepseek-pro-ga", "DeepSeek V4 Pro GA", "deepseek-v4-pro-ga-260813", "text.generate", "active", "text_to_text")
	add("ark-deepseek-flash-ga", "DeepSeek V4 Flash GA", "deepseek-v4-flash-ga-260731", "text.generate", "active", "text_to_text")
	add("ark-chat-pro-20-old", "Doubao Seed 2.0 Pro · 260215", "doubao-seed-2-0-pro-260215", "text.generate", "sunsetting", "text_to_text")
	add("ark-chat-lite-20-old", "Doubao Seed 2.0 Lite · 260215", "doubao-seed-2-0-lite-260215", "text.generate", "sunsetting", "text_to_text")
	add("ark-chat-mini-20-old", "Doubao Seed 2.0 Mini · 260215", "doubao-seed-2-0-mini-260215", "text.generate", "sunsetting", "text_to_text")
	add("ark-code-20-preview", "Doubao Seed 2.0 Code Preview", "doubao-seed-2-0-code-preview-260215", "text.generate", "sunsetting", "text_to_text")
	add("ark-deepseek-pro-old", "DeepSeek V4 Pro · 260425", "deepseek-v4-pro-260425", "text.generate", "sunsetting", "text_to_text")
	add("ark-image-pro-50", "Seedream 5.0 Pro", "doubao-seedream-5-0-pro-260628", "image.generate", "active", "text2image", "image2image")
	add("ark-image-flash-50", "Seedream 5.0 Flash", "doubao-seedream-5-0-flash-260915", "image.generate", "active", "text2image", "image2image")
	add("ark-image-lite-50", "Seedream 5.0 Lite", "doubao-seedream-5-0-260128", "image.generate", "sunsetting", "text2image", "image2image")
	add("ark-image", "Seedream 4.5", "doubao-seedream-4-5-251128", "image.generate", "sunsetting", "text2image", "image2image")
	add("ark-image-40", "Seedream 4.0", "doubao-seedream-4-0-250828", "image.generate", "sunsetting", "text2image", "image2image")
	add("ark-video-25", "Seedance 2.5", "doubao-seedance-2-5-260628", "video.generate", "active", "text2video", "image2video", "omni_reference")
	add("ark-video", "Seedance 2.0", "doubao-seedance-2-0-260128", "video.generate", "active", "text2video", "image2video", "omni_reference")
	add("ark-video-fast", "Seedance 2.0 Fast", "doubao-seedance-2-0-fast-260128", "video.generate", "active", "text2video", "image2video", "omni_reference")
	add("ark-video-mini", "Seedance 2.0 Mini", "doubao-seedance-2-0-mini-260615", "video.generate", "active", "text2video", "image2video", "omni_reference")
	add("ark-video-10", "Seedance 1.0 Pro", "doubao-seedance-1-0-pro-250528", "video.generate", "sunsetting", "text2video", "image2video")
	add("ark-video-10-fast", "Seedance 1.0 Pro Fast", "doubao-seedance-1-0-pro-fast-251015", "video.generate", "sunsetting", "text2video", "image2video")
	return catalog
}

func doubaoSpeechCatalog() ModelCandidateCatalog {
	return ModelCandidateCatalog{Version: "2026-10-29.1", Models: []ModelCandidatePreset{
		{Key: "speech-tts-20", AdapterKey: "volcengine_speech", DisplayName: "豆包语音合成 2.0", ProviderModelID: "seed-tts-2.0", Capability: "audio.generate", Modes: []string{"text2audio"}, SourceURL: "https://docs.volcengine.com/docs/doubao-voice/unidirectional-streaming-text-to-speech-http?lang=zh", ReviewedAt: "2026-10-28", Lifecycle: "active"},
		{Key: "speech-omni-10", AdapterKey: "volcengine_speech", DisplayName: "Seed-Audio 1.0 · 全能配音", ProviderModelID: "seed-audio-1.0", Capability: "audio.generate", Modes: []string{"omni_audio"}, SourceURL: "https://docs.volcengine.com/docs/doubao-voice/audio-generation-http?lang=zh", ReviewedAt: "2026-10-29", Lifecycle: "active"},
	}}
}

func configureSeedance25(version *ModelVersion) {
	version.Limits = json.RawMessage(`{"max_outputs":1,"roles":{"_total_images":30,"_total_videos":10,"_total_audios":10,"first_frame":{"max_count":1,"min_count":1,"types":["image"],"for_modes":["image2video"]},"last_frame":{"max_count":1,"types":["image"],"for_modes":["image2video"]},"reference_image":{"max_count":30,"types":["image"],"for_modes":["omni_reference"]},"reference_video":{"max_count":10,"types":["video"],"for_modes":["omni_reference"]},"reference_audio":{"max_count":10,"types":["audio"],"for_modes":["omni_reference"]},"subject":{"max_count":30,"types":["image","video","audio"],"for_modes":["omni_reference"]}}}`)
	version.ParamSchema = json.RawMessage(`[{"field":"duration_ms","label":"视频时长（毫秒）","component":"input","type":"integer","min":4000,"max":30000,"step":1000,"default":5000},{"field":"resolution","label":"分辨率","component":"select","type":"string","enum":["480p","720p","1080p"],"default":"720p"},{"field":"aspect_ratio","label":"画面比例","component":"select","type":"string","for_modes":["text2video","omni_reference"],"enum":["adaptive","16:9","9:16","1:1","4:3","3:4","21:9"],"default":"adaptive"},{"field":"generate_audio","label":"生成声音","component":"switch","type":"boolean","default":true}]`)
}

// configureSeedanceGeneration keeps older model limits distinct from 2.5.
func configureSeedanceGeneration(version *ModelVersion) {
	switch {
	case strings.HasPrefix(version.ProviderModelID, "doubao-seedance-2-5-"):
		configureSeedance25(version)
	case strings.HasPrefix(version.ProviderModelID, "doubao-seedance-1-0-"):
		version.ParamSchema = json.RawMessage(`[{"field":"duration_ms","label":"视频时长（毫秒）","component":"input","type":"integer","min":2000,"max":12000,"step":1000,"default":5000},{"field":"resolution","label":"分辨率","component":"select","type":"string","enum":["480p","720p","1080p"],"default":"720p"},{"field":"aspect_ratio","label":"画面比例","component":"select","type":"string","enum":["16:9","9:16","1:1"],"default":"16:9"}]`)
		if strings.HasPrefix(version.ProviderModelID, "doubao-seedance-1-0-pro-fast-") {
			version.Limits = json.RawMessage(`{"max_outputs":1,"roles":{"_total_images":1,"first_frame":{"max_count":1,"min_count":1,"types":["image"],"for_modes":["image2video"]}}}`)
		}
	case strings.HasPrefix(version.ProviderModelID, "doubao-seedance-2-0-fast-"), strings.HasPrefix(version.ProviderModelID, "doubao-seedance-2-0-mini-"):
		version.ParamSchema = json.RawMessage(`[{"field":"duration_ms","label":"视频时长（毫秒）","component":"input","type":"integer","min":4000,"max":15000,"step":1000,"default":5000},{"field":"resolution","label":"分辨率","component":"select","type":"string","enum":["480p","720p"],"default":"720p"},{"field":"aspect_ratio","label":"画面比例","component":"select","type":"string","enum":["16:9","9:16","1:1"],"default":"16:9"}]`)
	}
}
