package providerhttp

import (
	"encoding/json"
	"net/url"
	"slices"
	"strings"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func submission(input application.GenerationRequest) (string, map[string]any, bool) {
	if input.Config.Validate() != nil || !slices.Contains(input.Config.Capabilities, strings.TrimSuffix(input.Capability, ".generate")) || strings.TrimSpace(input.ModelID) == "" || len(input.ModelID) > 256 || strings.TrimSpace(input.Prompt) == "" || len(input.Prompt) > 65536 || input.OutputCount < 1 || input.OutputCount > 8 {
		return "", nil, false
	}
	payload := map[string]any{"model": input.ModelID, "prompt": input.Prompt}
	path := ""
	switch input.Capability {
	case "text.generate":
		if input.Config.AdapterKey != "openrouter" || input.OutputCount != 1 || len(input.References) != 0 {
			return "", nil, false
		}
		path = "/chat/completions"
		delete(payload, "prompt")
		payload["messages"] = []map[string]string{{"role": "user", "content": input.Prompt}}
		payload["stream"] = false
	case "image.generate":
		if input.Config.AdapterKey != "openrouter" {
			return "", nil, false
		}
		path = "/images"
		payload["n"] = input.OutputCount
	case "video.generate":
		if input.OutputCount != 1 {
			return "", nil, false
		}
		path = "/videos"
		if input.Config.AdapterKey == "volcengine_ark" {
			path = "/contents/generations/tasks"
		}
	default:
		return "", nil, false
	}
	if _, milliseconds := input.Params["duration_ms"]; milliseconds {
		if _, seconds := input.Params["duration"]; seconds {
			return "", nil, false
		}
	}
	if _, ratio := input.Params["ratio"]; ratio {
		if _, aspect := input.Params["aspect_ratio"]; aspect {
			return "", nil, false
		}
	}
	allowed := input.Config.ParameterFields(input.Capability)
	if len(allowed) == 0 {
		return "", nil, false
	}
	for key, value := range input.Params {
		if !slices.Contains(allowed, key) {
			return "", nil, false
		}
		switch {
		case key == "duration_ms":
			raw, err := json.Marshal(value)
			var milliseconds int64
			if err != nil || json.Unmarshal(raw, &milliseconds) != nil || milliseconds <= 0 || milliseconds%1000 != 0 {
				return "", nil, false
			}
			payload["duration"] = milliseconds / 1000
		case key == "aspect_ratio" && input.Config.AdapterKey == "volcengine_ark":
			payload["ratio"] = value
		default:
			payload[key] = value
		}
	}
	if input.EndpointTag != "" {
		if input.Config.AdapterKey != "openrouter" || input.Capability == "video.generate" || len(input.EndpointTag) > 128 {
			return "", nil, false
		}
		payload["provider"] = map[string]any{"only": []string{input.EndpointTag}, "allow_fallbacks": false, "require_parameters": true}
	} else if input.Capability == "text.generate" || input.Capability == "image.generate" {
		payload["provider"] = map[string]any{"allow_fallbacks": false, "require_parameters": true}
	}
	references := []map[string]any{}
	frames := []map[string]any{}
	arkContent := []map[string]any{{"type": "text", "text": input.Prompt}}
	seenFrames := map[string]bool{}
	for _, ref := range input.References {
		parsed, err := url.Parse(ref.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || len(ref.URL) > 8192 {
			return "", nil, false
		}
		item := map[string]any{"type": "image_url", "image_url": map[string]string{"url": ref.URL}}
		switch ref.Role {
		case "first_frame", "last_frame":
			if input.Capability != "video.generate" || seenFrames[ref.Role] {
				return "", nil, false
			}
			seenFrames[ref.Role] = true
			if input.Config.AdapterKey == "volcengine_ark" {
				item["role"] = ref.Role
			} else {
				item["frame_type"] = ref.Role
			}
			frames = append(frames, item)
		case "reference_image", "reference", "subject":
			if input.Config.AdapterKey == "volcengine_ark" {
				item["role"] = "reference_image"
			}
			references = append(references, item)
		default:
			return "", nil, false
		}
		arkContent = append(arkContent, item)
	}
	if len(frames) > 0 && len(references) > 0 || seenFrames["last_frame"] && !seenFrames["first_frame"] {
		return "", nil, false
	}
	if input.Config.AdapterKey == "volcengine_ark" {
		delete(payload, "prompt")
		payload["content"] = arkContent
	} else {
		if len(frames) > 0 {
			payload["frame_images"] = frames
		}
		if len(references) > 0 {
			payload["input_references"] = references
		}
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
		return "", nil, false
	}
	return path, payload, true
}
