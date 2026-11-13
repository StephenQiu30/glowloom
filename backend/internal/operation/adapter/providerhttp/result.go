package providerhttp

import (
	"encoding/json"
	"strings"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func unknownResult() application.GenerationResult {
	return application.GenerationResult{Outcome: application.ProviderOutcomeUnknown, State: application.ProviderTaskUnknown, ErrorCode: "provider_response_invalid"}
}

func invalidSubmissionResult() application.GenerationResult {
	return application.GenerationResult{Outcome: application.ProviderOutcomeNotSubmitted, ErrorCode: "invalid_provider_request"}
}

func safeHTTPError(status int) string {
	switch status {
	case 401, 403:
		return "auth_failed"
	case 402:
		return "payment_required"
	case 429:
		return "request_limited"
	case 400, 422:
		return "invalid_parameters"
	case 404:
		return "not_found"
	default:
		return "upstream_unavailable"
	}
}

// safeHTTPErrorBody maps only reviewed error codes; upstream messages never escape.
func safeHTTPErrorBody(status int, data []byte) string {
	var body struct {
		Error struct {
			Code string `json:"code"`
			Type string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &body) == nil {
		for _, code := range []string{body.Error.Code, body.Error.Type} {
			switch code {
			case "rate_limit_exceeded", "rate_limit_error":
				return "rate_limited"
			case "credit_balance_exhausted":
				return "insufficient_balance"
			case "project_spend_limit_exceeded":
				return "project_limit_exceeded"
			case "organization_usage_limit_exceeded":
				return "organization_limit_exceeded"
			case "insufficient_quota":
				return "insufficient_quota"
			case "ip_not_authorized":
				return "ip_restricted"
			case "unsupported_country_region_territory":
				return "region_restricted"
			case "permission_denied", "insufficient_permissions":
				return "permission_denied"
			case "invalid_api_key":
				return "auth_failed"
			}
		}
	}
	if status == 403 {
		return "permission_denied"
	}
	return safeHTTPError(status)
}

// Ark uses HTTP 400 for both moderation and malformed parameters.
func arkSubmitError(status int, data []byte) string {
	fallback := safeHTTPErrorBody(status, data)
	if fallback != "invalid_parameters" {
		return fallback
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &body) == nil {
		code, _, _ := strings.Cut(body.Error.Code, ".")
		switch code {
		case "InputTextSensitiveContentDetected", "InputImageSensitiveContentDetected", "InputVideoSensitiveContentDetected", "InputAudioSensitiveContentDetected":
			return "input_content_rejected"
		case "OutputImageSensitiveContentDetected", "OutputTextSensitiveContentDetected", "OutputVideoSensitiveContentDetected", "OutputAudioSensitiveContentDetected":
			return "output_content_rejected"
		case "InvalidParameter", "MissingParameter":
			return "invalid_parameters"
		}
	}
	return "request_rejected"
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
	// Preserve only the reviewed cache count, never arbitrary nested usage data.
	var cached *int64
	addCached := func(raw json.RawMessage) bool {
		var count int64
		if json.Unmarshal(raw, &count) != nil || count < 0 || (cached != nil && *cached != count) {
			return false
		}
		cached = &count
		return true
	}
	if raw, ok := fields["prompt_cache_hit_tokens"]; ok && !addCached(raw) {
		return nil
	}
	for _, name := range []string{"prompt_tokens_details", "input_tokens_details"} {
		if raw, ok := fields[name]; ok {
			var detail map[string]json.RawMessage
			if json.Unmarshal(raw, &detail) != nil {
				return nil
			}
			if count, present := detail["cached_tokens"]; present && !addCached(count) {
				return nil
			}
		}
	}
	if cached != nil {
		raw, _ := json.Marshal(*cached)
		allowed["cached_input_tokens"] = raw
	}
	if len(allowed) == 0 {
		return nil
	}
	data, _ := json.Marshal(allowed)
	return data
}
