package domain

// ParameterFields is shared by preset validation and execution so a preset
// cannot advertise parameters that the deployed protocol refuses to submit.
// It returns fresh slices and never admits credential or routing headers.
func (c ProviderConfig) ParameterFields(capability string) []string {
	if c.AdapterKey == "volcengine_music" && capability == "audio.generate" {
		return []string{"duration_ms", "music_input"}
	}
	if c.AdapterKey == "volcengine_avatar" && capability == "video.generate" {
		return []string{"output_resolution", "pe_fast_mode"}
	}
	if c.AdapterKey == "volcengine_motion" && capability == "video.generate" {
		return []string{"cut_result_first_second_switch"}
	}

	if c.AdapterKey == "volcengine_speech" && capability == "audio.generate" {
		return []string{"speaker", "speech_rate", "loudness_rate", "pitch_rate", "enable_subtitle"}
	}
	if capability == "text.generate" && IsManagedCloud(c.AdapterKey) {
		return []string{"temperature", "top_p", "max_tokens", "seed", "response_format"}
	}
	if capability == "image.generate" && (c.AdapterKey == "volcengine_ark" || c.AdapterKey == "byteplus") {
		return []string{"size", "seed", "watermark"}
	}
	if c.AdapterKey == "volcengine_ark" && capability == "video.generate" {
		return []string{"duration", "duration_ms", "resolution", "ratio", "aspect_ratio", "generate_audio", "seed", "watermark", "camera_fixed"}
	}
	if c.AdapterKey != "openrouter" {
		return nil
	}
	switch capability {
	case "text.generate":
		return []string{"temperature", "top_p", "max_tokens", "seed", "response_format"}
	case "image.generate":
		return []string{"resolution", "aspect_ratio", "quality", "output_format", "background", "seed", "output_compression"}
	case "video.generate":
		return []string{"duration", "duration_ms", "resolution", "aspect_ratio", "generate_audio", "seed"}
	default:
		return nil
	}
}

// IsManagedCloud identifies protocols that require a frozen catalog release.
func IsManagedCloud(adapter string) bool {
	switch adapter {
	case "volcengine_ark", "openrouter", "byteplus", "deepseek", "volcengine_speech", "volcengine_music", "volcengine_avatar", "volcengine_motion":
		return true
	default:
		return false
	}
}
