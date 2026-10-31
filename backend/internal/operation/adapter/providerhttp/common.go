package providerhttp

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"net/http"
	"slices"
	"strings"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func unknownResult() application.GenerationResult {
	return application.GenerationResult{Outcome: "unknown", State: "unknown", ErrorCode: "provider_response_invalid"}
}

func invalidSubmissionResult() application.GenerationResult {
	return application.GenerationResult{Outcome: "not_submitted", ErrorCode: "invalid_provider_request"}
}

func validKey(key string) bool {
	return key != "" && len(key) <= 4096 && !strings.ContainsAny(key, "\r\n\x00")
}

func validTaskID(id string) bool {
	if len(id) == 0 || len(id) > 256 {
		return false
	}
	for _, c := range id {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' && c != '_' {
			return false
		}
	}
	return true
}

func safeHTTPError(status int) string {
	switch status {
	case 401, 403:
		return "auth_failed"
	case 402:
		return "insufficient_balance"
	case 429:
		return "rate_limited"
	case 400, 422:
		return "invalid_parameters"
	case 404:
		return "not_found"
	default:
		return "upstream_unavailable"
	}
}

func safeUsage(raw json.RawMessage) json.RawMessage {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil
	}
	allowed := map[string]json.RawMessage{}
	for _, key := range []string{"cost", "prompt_tokens", "completion_tokens", "total_tokens", "input_tokens", "output_tokens"} {
		if value, ok := fields[key]; ok {
			var number json.Number
			if json.Unmarshal(value, &number) == nil {
				if v, err := number.Float64(); err == nil && v >= 0 {
					allowed[key] = value
				}
			}
		}
	}
	if len(allowed) == 0 {
		return nil
	}
	data, _ := json.Marshal(allowed)
	return data
}

func integerParam(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		if math.Floor(v) == v {
			return int64(v), true
		}
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return n, true
		}
	}
	return 0, false
}

func validateCommonInput(input application.GenerationRequest) bool {
	if input.Config.Validate() != nil ||
		!slices.Contains(input.Config.Capabilities, strings.TrimSuffix(input.Capability, ".generate")) ||
		strings.TrimSpace(input.ModelID) == "" || len(input.ModelID) > 256 ||
		strings.TrimSpace(input.Prompt) == "" || len(input.Prompt) > 65536 ||
		input.OutputCount < 1 || input.OutputCount > 8 {
		return false
	}
	return true
}

func applyAllowedParameters(input application.GenerationRequest, payload map[string]any, mapAspectRatioToRatio bool) bool {
	if _, milliseconds := input.Params["duration_ms"]; milliseconds {
		if _, seconds := input.Params["duration"]; seconds {
			return false
		}
	}
	if _, ratio := input.Params["ratio"]; ratio {
		if _, aspect := input.Params["aspect_ratio"]; aspect {
			return false
		}
	}
	allowed := input.Config.ParameterFields(input.Capability)
	if len(allowed) == 0 {
		return false
	}
	for key, value := range input.Params {
		if !slices.Contains(allowed, key) {
			return false
		}
		switch {
		case key == "duration_ms":
			raw, err := json.Marshal(value)
			var milliseconds int64
			if err != nil || json.Unmarshal(raw, &milliseconds) != nil || milliseconds <= 0 || milliseconds%1000 != 0 {
				return false
			}
			payload["duration"] = milliseconds / 1000
		case key == "aspect_ratio" && mapAspectRatioToRatio:
			payload["ratio"] = value
		default:
			payload[key] = value
		}
	}
	return true
}

func parseStandardGenerationResponse(input application.GenerationRequest, data []byte) (application.GenerationResult, bool) {
	var response struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Data []struct {
			Base64    string `json:"b64_json"`
			MediaType string `json:"media_type"`
		} `json:"data"`
		Usage json.RawMessage `json:"usage"`
	}
	if json.Unmarshal(data, &response) != nil {
		return unknownResult(), false
	}
	result := application.GenerationResult{Outcome: "completed", State: "succeeded", Usage: safeUsage(response.Usage)}
	switch input.Capability {
	case "video.generate":
		if !validTaskID(response.ID) {
			return unknownResult(), false
		}
		return application.GenerationResult{Outcome: "accepted", State: "pending", TaskID: response.ID}, true
	case "text.generate":
		if len(response.Choices) != 1 || response.Choices[0].Message.Content == "" || response.Choices[0].FinishReason != "stop" {
			return unknownResult(), false
		}
		result.Text = response.Choices[0].Message.Content
	case "image.generate":
		if len(response.Data) != input.OutputCount {
			return unknownResult(), false
		}
		for _, image := range response.Data {
			decoded, err := base64.StdEncoding.DecodeString(image.Base64)
			if err != nil || len(decoded) == 0 || len(decoded) > 32<<20 {
				return unknownResult(), false
			}
			kind := http.DetectContentType(decoded)
			if !slices.Contains([]string{"image/png", "image/jpeg", "image/webp"}, kind) || (image.MediaType != "" && image.MediaType != kind) {
				return unknownResult(), false
			}
			result.Images = append(result.Images, application.GeneratedImage{Bytes: decoded, MediaType: kind})
		}
	default:
		return unknownResult(), false
	}
	return result, true
}
