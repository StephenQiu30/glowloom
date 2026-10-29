package domain

import "encoding/json"

func configureOmniAudioVersion(m ModelCandidatePreset, v *ModelVersion) bool {
	if m.AdapterKey != "volcengine_speech" || m.ProviderModelID != "seed-audio-1.0" {
		return false
	}
	v.Limits = json.RawMessage(`{"max_outputs":1,"durations_ms":[120000],"roles":{"reference_image":{"max_count":1,"types":["image"],"max_bytes":10000000},"reference_audio":{"max_count":3,"types":["audio"],"max_bytes":10000000,"max_ms":30000}}}`)
	v.ParamSchema = json.RawMessage(`[{"field":"speech_rate","label":"语速","component":"input","type":"integer","min":-50,"max":100,"default":0},{"field":"loudness_rate","label":"音量","component":"input","type":"integer","min":-50,"max":100,"default":0},{"field":"pitch_rate","label":"音调","component":"input","type":"integer","min":-12,"max":12,"default":0},{"field":"enable_subtitle","label":"生成字幕","component":"switch","type":"boolean","default":true}]`)
	v.ExpectedMaxMS = 300000
	return true
}
