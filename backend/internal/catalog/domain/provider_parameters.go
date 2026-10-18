package domain

// ParameterFields is shared by preset validation and execution so a preset
// cannot advertise parameters that the deployed protocol refuses to submit.
// It returns fresh slices and never admits credential or routing headers.
func (c ProviderConfig) ParameterFields(capability string) []string {
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
