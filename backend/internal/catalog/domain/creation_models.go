package domain

import "encoding/json"

func creationModelCatalog(adapter string) ModelCandidateCatalog {
	m := ModelCandidatePreset{AdapterKey: adapter, ReviewedAt: "2026-10-29", Lifecycle: "active"}
	one := 1
	m.MaxOutputs = &one
	switch adapter {
	case "volcengine_music":
		m.Key, m.DisplayName, m.ProviderModelID, m.Capability = "music-v5", "火山音乐 v5", "v5.0", "audio.generate"
		m.Modes = []string{"music_song", "music_instrumental"}
		m.SourceURL = "https://docs.volcengine.com/docs/Audiovideounderstandingandprocessing/Generatevocalsongs?lang=zh"
	case "volcengine_avatar":
		m.Key, m.DisplayName, m.ProviderModelID, m.Capability = "omnihuman-v15", "OmniHuman 1.5", "jimeng_realman_avatar_picture_omni_v15", "video.generate"
		m.Modes = []string{"avatar"}
		m.SourceURL = "https://docs.volcengine.com/docs/jimeng-ai/call-step3-video-generation?lang=zh"
	case "volcengine_motion":
		m.Key, m.DisplayName, m.ProviderModelID, m.Capability = "dreamactor-m20", "动作模仿 2.0", "jimeng_dreamactor_m20_gen_video", "video.generate"
		m.Modes = []string{"motion"}
		m.SourceURL = "https://docs.volcengine.com/docs/jimeng-ai/action-imitation-20-interface-documentation?lang=zh"
	}
	return ModelCandidateCatalog{Version: "2026-10-29.1", Models: []ModelCandidatePreset{m}}
}

func configureCreationVersion(m ModelCandidatePreset, v *ModelVersion) bool {
	var maxSeconds int
	switch m.AdapterKey {
	case "volcengine_music":
		v.ParamSchema = json.RawMessage(`[{"field":"duration_ms","label":"时长","component":"input","type":"integer","required":true,"min":30000,"max":240000,"step":1000,"default":60000},{"field":"music_input","label":"歌曲输入","component":"select","type":"string","enum":["description","lyrics"],"default":"description","for_modes":["music_song"]}]`)
		maxSeconds = 240
	case "volcengine_avatar":
		v.ParamSchema = json.RawMessage(`[{"field":"output_resolution","label":"分辨率","component":"select","type":"integer","enum":[720,1080],"default":1080},{"field":"pe_fast_mode","label":"快速模式","component":"switch","type":"boolean","default":false}]`)
		v.Limits = json.RawMessage(`{"max_outputs":1,"roles":{"character_image":{"min_count":1,"max_count":1,"types":["image"],"max_bytes":4999999},"driving_audio":{"min_count":1,"max_count":1,"types":["audio"],"max_ms":59999}}}`)
		maxSeconds = 60
	case "volcengine_motion":
		v.ParamSchema = json.RawMessage(`[{"field":"cut_result_first_second_switch","label":"裁剪首秒","component":"switch","type":"boolean","default":true}]`)
		v.Limits = json.RawMessage(`{"max_outputs":1,"roles":{"character_image":{"min_count":1,"max_count":1,"types":["image"],"max_bytes":4699999},"driving_video":{"min_count":1,"max_count":1,"types":["video"],"max_ms":30000}}}`)
		maxSeconds = 30
	default:
		return false
	}
	// Per-second quote admission uses whole-second ceilings of frozen input
	// duration. Supplier receipts keep their independent original precision.
	var limits map[string]any
	_ = json.Unmarshal(v.Limits, &limits)
	steps := make([]int, maxSeconds)
	for i := range steps {
		steps[i] = (i + 1) * 1000
	}
	limits["durations_ms"] = steps
	v.Limits, _ = json.Marshal(limits)
	v.SupportsQuery = true
	v.ExpectedMaxMS = 3600000
	return true
}
