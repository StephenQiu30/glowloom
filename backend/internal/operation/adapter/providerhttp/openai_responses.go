package providerhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// submitOpenAIResponses performs one stateless text generation. Unknown outcomes
// must be reconciled by the operation owner rather than resubmitted here.
func submitOpenAIResponses(ctx context.Context, c *Client, input application.GenerationRequest, key string) (application.GenerationResult, error) {
	if input.OutputCount != 1 || len(input.References) != 0 || input.Mode != "text_to_text" || input.EndpointTag != "" {
		return invalidSubmissionResult(), nil
	}
	// Keep legacy parameter fields available for frozen Chat Completions
	// versions, but do not forward them to current reasoning models.
	for field := range input.Params {
		if field != "max_tokens" {
			return invalidSubmissionResult(), nil
		}
	}
	payload := map[string]any{"model": input.ModelID, "input": input.Prompt, "store": false, "stream": false}
	if !applyAllowedParameters(input, payload, false) {
		return invalidSubmissionResult(), nil
	}
	if limit, ok := payload["max_tokens"]; ok {
		payload["max_output_tokens"] = limit
		delete(payload, "max_tokens")
	}
	data, status, err := c.request(ctx, input.Config, key, http.MethodPost, "/responses", payload, 8<<20)
	if err != nil {
		return application.GenerationResult{Outcome: application.ProviderOutcomeUnknown, ErrorCode: "transport_unknown"}, nil
	}
	if status < 200 || status >= 300 {
		return application.GenerationResult{Outcome: application.ProviderOutcomeUnknown, ErrorCode: safeHTTPErrorBody(status, data)}, nil
	}
	var response struct {
		Status string `json:"status"`
		Output []struct {
			Type    string `json:"type"`
			Status  string `json:"status"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage json.RawMessage `json:"usage"`
	}
	if json.Unmarshal(data, &response) != nil || response.Status != "completed" {
		return unknownResult(), nil
	}
	var output strings.Builder
	for _, item := range response.Output {
		if item.Type != "message" {
			continue
		}
		for _, part := range item.Content {
			if part.Type == "refusal" {
				return application.GenerationResult{Outcome: application.ProviderOutcomeUnknown, ErrorCode: "content_rejected", Usage: safeUsage(response.Usage)}, nil
			}
			if item.Status != "completed" {
				return unknownResult(), nil
			}
			if part.Type == "output_text" {
				output.WriteString(part.Text)
			}
		}
	}
	if strings.TrimSpace(output.String()) == "" {
		return unknownResult(), nil
	}
	return application.GenerationResult{Outcome: application.ProviderOutcomeCompleted, State: application.ProviderTaskSucceeded, Text: output.String(), Usage: safeUsage(response.Usage)}, nil
}
