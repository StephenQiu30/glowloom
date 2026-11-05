package providerhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

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

	authData, status, err := c.request(ctx, config, key, http.MethodGet, "/models", nil, 1<<20)
	if err != nil {
		result.Auth = "unreachable"
		return result
	}
	result.HTTPStatus = status
	if status != 200 {
		result.Auth = safeHTTPErrorBody(status, authData)
		return result
	}
	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(authData, &response) != nil || response.Data == nil || len(response.Data) > 500 {
		result.Auth, result.Discovery = "invalid_response", "invalid_response"
		return result
	}
	for _, model := range response.Data {
		if model.ID == "" || len(model.ID) > 256 {
			result.Auth, result.Discovery = "invalid_response", "invalid_response"
			return result
		}
		capability := "text.generate"
		if strings.HasPrefix(model.ID, "gpt-image-") || model.ID == "chatgpt-image-latest" || model.ID == "dall-e-3" || model.ID == "dall-e-2" {
			capability = "image.generate"
		}
		result.Candidates = append(result.Candidates, catalogapp.ModelCandidate{
			ProviderModelID: model.ID,
			DisplayName:     model.ID,
			Capability:      capability,
			Metadata:        json.RawMessage(`{"verified":false}`),
		})
	}
	result.Auth, result.Discovery = "ok", "ok"
	return result
}

// ProbeModelDefaultParams applies this adapter's model-probe parameter defaults.
func (s *VercelStrategy) ProbeModelDefaultParams(capability string) map[string]any {
	if capability == "image.generate" {
		return map[string]any{"size": "1024x1024"}
	}
	return nil
}
