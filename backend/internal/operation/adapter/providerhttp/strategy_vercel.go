package providerhttp

import (
	"context"
	"encoding/json"
	"net/http"

	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// VercelStrategy implements ProtocolStrategy for Vercel AI Gateway endpoints.
type VercelStrategy struct{}

// Name returns the adapter protocol key.
func (s *VercelStrategy) Name() string {
	return "vercel"
}

// Submit sends generation input using this adapter's provider protocol.
func (s *VercelStrategy) Submit(ctx context.Context, c *Client, input application.GenerationRequest, key string) (application.GenerationResult, error) {
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
		if input.OutputCount != 1 || len(input.References) != 0 || input.Mode != "text2image" {
			return invalidSubmissionResult(), nil
		}
		if isGPTImage(input.ModelID) {
			for field := range input.Params {
				if field != "size" && field != "quality" {
					return invalidSubmissionResult(), nil
				}
			}
		}
		path = "/images/generations"
		if !isGPTImage(input.ModelID) {
			payload["response_format"] = "b64_json"
		}
	default:
		return invalidSubmissionResult(), nil
	}

	if !applyAllowedParameters(input, payload, false) {
		return invalidSubmissionResult(), nil
	}

	validMode := false
	switch input.Capability {
	case "text.generate":
		validMode = input.Mode == "text_to_text"
	case "image.generate":
		validMode = input.Mode == "text2image"
	}
	if !validMode {
		return invalidSubmissionResult(), nil
	}

	data, status, err := c.request(ctx, input.Config, key, http.MethodPost, path, payload, 64<<20)
	if err != nil {
		return application.GenerationResult{Outcome: application.ProviderOutcomeUnknown, ErrorCode: "transport_unknown"}, nil
	}
	if status < 200 || status >= 300 {
		return application.GenerationResult{Outcome: application.ProviderOutcomeUnknown, ErrorCode: safeHTTPErrorBody(status, data)}, nil
	}
	result, ok := parseStandardGenerationResponse(input, data)
	if !ok {
		return unknownResult(), nil
	}
	return result, nil
}

// Query reads provider progress or reports that polling is unsupported.
func (s *VercelStrategy) Query(_ context.Context, _ *Client, _ catalogdomain.ProviderConfig, _, _ string) (application.GenerationResult, error) {
	return unknownResult(), nil
}

// Probe performs the configured provider credential check.
func (s *VercelStrategy) Probe(ctx context.Context, c *Client, config catalogdomain.ProviderConfig, key string) ProbeResult {
	result := ProbeResult{Auth: "unknown", Adapter: "ok", Discovery: "unsupported", Candidates: []catalogapp.ModelCandidate{}}
	if config.Validate() != nil || !validKey(key) {
		result.Adapter = "unsupported"
		return result
	}

	// The public model catalog does not authenticate the key.
	body, status, err := c.request(ctx, config, key, http.MethodGet, "/credits", nil, 16<<10)
	if err != nil {
		result.Auth = "unreachable"
		return result
	}
	result.HTTPStatus = status
	if status != http.StatusOK {
		result.Auth = safeHTTPErrorBody(status, body)
		return result
	}
	var credits struct {
		Balance   string `json:"balance"`
		TotalUsed string `json:"total_used"`
	}
	if json.Unmarshal(body, &credits) != nil || credits.Balance == "" || credits.TotalUsed == "" {
		result.Auth = "invalid_response"
		return result
	}
	result.Auth = "ok"
	return result
}

// ProbeModelDefaultParams applies this adapter's model-probe parameter defaults.
func (s *VercelStrategy) ProbeModelDefaultParams(capability string) map[string]any {
	if capability == "image.generate" {
		return map[string]any{"size": "1024x1024"}
	}
	return nil
}
