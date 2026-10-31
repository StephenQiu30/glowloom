package providerhttp

import (
	"encoding/json"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func unknownResult() application.GenerationResult {
	return application.GenerationResult{Outcome: "unknown", State: "unknown", ErrorCode: "provider_response_invalid"}
}

func invalidSubmissionResult() application.GenerationResult {
	return application.GenerationResult{Outcome: "not_submitted", ErrorCode: "invalid_provider_request"}
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
