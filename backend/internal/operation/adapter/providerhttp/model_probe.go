package providerhttp

import (
	"context"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// ModelProbeResult records only safe access evidence. An accepted video task is
// explicitly distinct from a completed generation; no generated content is kept.
type ModelProbeResult struct {
	Result    string
	ErrorCode string
	TaskID    string
}

// ProbeModel sends exactly one bounded real request. Callers must durably claim
// dispatch before calling, and must never retry an unknown result automatically.
func (c *Client) ProbeModel(ctx context.Context, config catalogdomain.ProviderConfig, model, capability, key string) ModelProbeResult {
	input := application.GenerationRequest{Config: config, ModelID: model, Capability: capability, OutputCount: 1, Prompt: "Reply with OK only.", Mode: "text_to_text", Params: map[string]any{"max_tokens": 4096}}
	switch capability {
	case "audio.generate":
		input.Mode, input.Prompt = "text2audio", "你好。"
		input.Params = map[string]any{"speaker": "zh_female_vv_uranus_bigtts"}
	case "image.generate":
		input.Mode, input.Prompt = "text2image", "A small blue circle on a white background."
		input.Params = map[string]any{}
		if config.AdapterKey == "volcengine_ark" {
			input.Params["size"] = "2K"
		}
	case "video.generate":
		input.Mode, input.Prompt = "text2video", "A blue circle slowly moving on a white background."
		input.Params = map[string]any{"duration": 4, "resolution": "720p"}
	}
	generated, err := c.Submit(ctx, input, key)
	if err != nil {
		return ModelProbeResult{Result: "unknown", ErrorCode: "unreachable"}
	}
	switch generated.Outcome {
	case "completed":
		return ModelProbeResult{Result: "completed"}
	case "accepted":
		return ModelProbeResult{Result: "accepted", TaskID: generated.TaskID}
	case "rejected", "not_submitted":
		return ModelProbeResult{Result: "rejected", ErrorCode: generated.ErrorCode}
	default:
		return ModelProbeResult{Result: "unknown", ErrorCode: "unreachable"}
	}
}
