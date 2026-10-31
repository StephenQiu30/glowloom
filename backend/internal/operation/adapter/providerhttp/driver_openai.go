package providerhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"

	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// OpenAICompatibleDriver implements the communication strategy for OpenAI-compatible
// providers including OpenAI official, DeepSeek, Codex, and OpenRouter.
type OpenAICompatibleDriver struct{}

func init() {
	RegisterDriver(&OpenAICompatibleDriver{}, "openai", "deepseek", "codex", "openrouter")
}

func (d *OpenAICompatibleDriver) Submit(ctx context.Context, c *Client, input application.GenerationRequest, key string) (application.GenerationResult, error) {
	if !validKey(key) || !validateCommonInput(input) {
		return invalidSubmissionResult(), nil
	}
	payload := map[string]any{"model": input.ModelID, "prompt": input.Prompt}
	path := ""
	switch input.Capability {
	case "text.generate":
		if input.OutputCount != 1 || len(input.References) != 0 || input.Mode != "text_to_text" {
			return invalidSubmissionResult(), nil
		}
		path = "/chat/completions"
		delete(payload, "prompt")
		payload["messages"] = []map[string]string{{"role": "user", "content": input.Prompt}}
		payload["stream"] = false
	case "image.generate":
		if input.Config.AdapterKey == "openai" {
			if input.OutputCount != 1 || len(input.References) != 0 || input.Mode != "text2image" {
				return invalidSubmissionResult(), nil
			}
			path = "/images/generations"
			payload["response_format"] = "b64_json"
		} else if input.Config.AdapterKey == "openrouter" {
			path = "/images"
			payload["n"] = input.OutputCount
		} else {
			return invalidSubmissionResult(), nil
		}
	case "video.generate":
		if input.OutputCount != 1 || input.Config.AdapterKey != "openrouter" {
			return invalidSubmissionResult(), nil
		}
		path = "/videos"
	default:
		return invalidSubmissionResult(), nil
	}

	if !applyAllowedParameters(input, payload, false) {
		return invalidSubmissionResult(), nil
	}

	if input.EndpointTag != "" {
		if input.Config.AdapterKey != "openrouter" || input.Capability == "video.generate" || len(input.EndpointTag) > 128 {
			return invalidSubmissionResult(), nil
		}
		payload["provider"] = map[string]any{"only": []string{input.EndpointTag}, "allow_fallbacks": false, "require_parameters": true}
	} else if input.Config.AdapterKey == "openrouter" && (input.Capability == "text.generate" || input.Capability == "image.generate") {
		payload["provider"] = map[string]any{"allow_fallbacks": false, "require_parameters": true}
	}

	frames := []map[string]any{}
	references := []map[string]any{}
	seenFrames := map[string]bool{}
	for _, ref := range input.References {
		parsed, err := url.Parse(ref.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || len(ref.URL) > 8192 {
			return invalidSubmissionResult(), nil
		}
		if ref.MediaKind != "" && ref.MediaKind != "image" {
			return invalidSubmissionResult(), nil
		}
		item := map[string]any{"type": "image_url", "image_url": map[string]string{"url": ref.URL}}
		switch ref.Role {
		case "first_frame", "last_frame":
			if input.Capability != "video.generate" || seenFrames[ref.Role] {
				return invalidSubmissionResult(), nil
			}
			seenFrames[ref.Role] = true
			item["frame_type"] = ref.Role
			frames = append(frames, item)
		case "reference_image", "reference", "subject":
			references = append(references, item)
		default:
			return invalidSubmissionResult(), nil
		}
	}
	if len(frames) > 0 && len(references) > 0 || seenFrames["last_frame"] && !seenFrames["first_frame"] {
		return invalidSubmissionResult(), nil
	}
	if len(frames) > 0 {
		payload["frame_images"] = frames
	}
	if len(references) > 0 {
		payload["input_references"] = references
	}

	validMode := false
	switch input.Capability {
	case "text.generate":
		validMode = input.Mode == "text_to_text"
	case "image.generate":
		validMode = input.Mode == "text2image" && len(references) == 0 || input.Mode == "image2image" && len(references) > 0
	case "video.generate":
		validMode = input.Mode == "text2video" && len(input.References) == 0 || input.Mode == "image2video" && seenFrames["first_frame"] || input.Mode == "omni_reference" && len(references) > 0
	}
	if !validMode {
		return invalidSubmissionResult(), nil
	}

	data, status, err := c.request(ctx, input.Config, key, http.MethodPost, path, payload, 64<<20)
	if err != nil {
		return application.GenerationResult{Outcome: "unknown", ErrorCode: "transport_unknown"}, nil
	}
	if status < 200 || status >= 300 {
		outcome := "unknown"
		if slices.Contains([]int{400, 401, 402, 403, 404, 422, 429}, status) {
			outcome = "rejected"
		}
		return application.GenerationResult{Outcome: outcome, ErrorCode: safeHTTPError(status)}, nil
	}
	result, ok := parseStandardGenerationResponse(input, data)
	if !ok {
		return unknownResult(), nil
	}
	return result, nil
}

func (d *OpenAICompatibleDriver) Query(ctx context.Context, c *Client, config catalogdomain.ProviderConfig, taskID, key string) (application.GenerationResult, error) {
	if config.Validate() != nil || config.AdapterKey != "openrouter" || !validKey(key) || !validTaskID(taskID) {
		return unknownResult(), nil
	}
	path := "/videos/" + taskID
	data, status, err := c.request(ctx, config, key, http.MethodGet, path, nil, 1<<20)
	if err != nil {
		return application.GenerationResult{State: "unknown", ErrorCode: "query_unreachable"}, nil
	}
	if status < 200 || status >= 300 {
		state := "unknown"
		if status == 404 {
			state = "not_found"
		}
		return application.GenerationResult{State: state, ErrorCode: safeHTTPError(status)}, nil
	}
	var response struct {
		ID           string          `json:"id"`
		Status       string          `json:"status"`
		UnsignedURLs []string        `json:"unsigned_urls"`
		Usage        json.RawMessage `json:"usage"`
	}
	if json.Unmarshal(data, &response) != nil || response.ID != taskID {
		return unknownResult(), nil
	}
	result := application.GenerationResult{TaskID: taskID, Usage: safeUsage(response.Usage)}
	switch response.Status {
	case "queued", "pending":
		result.State = "pending"
	case "running", "in_progress":
		result.State = "running"
	case "failed", "expired":
		result.State = "failed"
		result.ErrorCode = "generation_failed"
	case "cancelled":
		result.State = "cancelled"
	case "completed", "succeeded":
		result.State = "succeeded"
		if len(response.UnsignedURLs) != 1 {
			return unknownResult(), nil
		}
		expected := config.BaseURL + "/videos/" + taskID + "/content?index=0"
		if response.UnsignedURLs[0] != expected {
			return unknownResult(), nil
		}
		result.URLs = response.UnsignedURLs
	default:
		return unknownResult(), nil
	}
	return result, nil
}

func (d *OpenAICompatibleDriver) Probe(ctx context.Context, c *Client, config catalogdomain.ProviderConfig, key string) ProbeResult {
	result := ProbeResult{Auth: "unknown", Adapter: "ok", Discovery: "unsupported", Candidates: []catalogapp.ModelCandidate{}}
	if config.Validate() != nil || !validKey(key) {
		result.Adapter = "unsupported"
		return result
	}
	if config.AdapterKey == "deepseek" || config.AdapterKey == "openai" {
		authData, status, err := c.request(ctx, config, key, http.MethodGet, "/models", nil, 1<<20)
		if err != nil {
			result.Auth = "unreachable"
			return result
		}
		if status != 200 {
			result.Auth = safeHTTPError(status)
			return result
		}
		result.Auth = "ok"
		var response struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if json.Unmarshal(authData, &response) != nil || response.Data == nil || len(response.Data) > 500 {
			result.Discovery = "invalid_response"
			return result
		}
		for _, model := range response.Data {
			if model.ID == "" || len(model.ID) > 256 {
				result.Discovery = "invalid_response"
				return result
			}
			cap := "text.generate"
			if model.ID == "dall-e-3" || model.ID == "dall-e-2" {
				cap = "image.generate"
			}
			result.Candidates = append(result.Candidates, catalogapp.ModelCandidate{
				ProviderModelID: model.ID,
				DisplayName:     model.ID,
				Capability:      cap,
				Metadata:        json.RawMessage(`{"verified":false}`),
			})
		}
		result.Discovery = "ok"
		return result
	}
	if config.AdapterKey == "codex" {
		result.Auth = "ok"
		return result
	}

	// OpenRouter discovery: probe /key then enumerate endpoints.
	_, status, err := c.request(ctx, config, key, http.MethodGet, "/key", nil, 1<<20)
	if err != nil {
		result.Auth = "unreachable"
		return result
	}
	if status != 200 {
		result.Auth = safeHTTPError(status)
		return result
	}
	result.Auth = "ok"

	for _, entry := range []struct{ Path, Capability string }{
		{"/models", "text.generate"},
		{"/images/models", "image.generate"},
		{"/videos/models", "video.generate"},
	} {
		data, status, err := c.request(ctx, config, key, http.MethodGet, entry.Path, nil, 8<<20)
		if err != nil || status != 200 {
			result.Discovery = "unreachable"
			return result
		}
		var response struct {
			Data []struct {
				ID                  string          `json:"id"`
				Name                string          `json:"name"`
				Architecture        json.RawMessage `json:"architecture"`
				SupportedParameters json.RawMessage `json:"supported_parameters"`
				Pricing             json.RawMessage `json:"pricing"`
			} `json:"data"`
		}
		if json.Unmarshal(data, &response) != nil || len(response.Data) > 5000 {
			result.Discovery = "invalid_response"
			return result
		}
		if len(response.Data) > 200 {
			response.Data = response.Data[:200]
			result.Discovery = "partial"
		}
		for _, model := range response.Data {
			if entry.Capability == "text.generate" {
				var architecture struct {
					OutputModalities []string `json:"output_modalities"`
				}
				if json.Unmarshal(model.Architecture, &architecture) != nil || !slices.Contains(architecture.OutputModalities, "text") {
					continue
				}
			}
			if model.ID == "" || len(model.ID) > 256 || len(model.Name) > 300 {
				continue
			}
			metadata, _ := json.Marshal(map[string]any{
				"architecture":         model.Architecture,
				"supported_parameters": model.SupportedParameters,
				"pricing":              model.Pricing,
				"verified":             false,
			})
			if len(metadata) > 2048 {
				continue
			}
			result.Candidates = append(result.Candidates, catalogapp.ModelCandidate{
				ProviderModelID: model.ID,
				DisplayName:     model.Name,
				Capability:      entry.Capability,
				Metadata:        metadata,
			})
		}
	}
	if result.Discovery != "partial" {
		result.Discovery = "ok"
	}
	return result
}

func (d *OpenAICompatibleDriver) ProbeModelDefaultParams(capability string) map[string]any {
	if capability == "image.generate" {
		return map[string]any{"size": "1024x1024"}
	}
	return nil
}
