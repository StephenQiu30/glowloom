package providerhttp

import (
	"encoding/json"
	"math"
	"slices"
	"strings"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

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
