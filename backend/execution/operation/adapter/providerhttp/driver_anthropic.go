package providerhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	catalog "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
)

// AnthropicDriver supports native, stateless text Messages. It does not claim
// cross-protocol, thinking, cached writes, tools or streaming compatibility.
type AnthropicDriver struct{}

func init() { RegisterDriver(&AnthropicDriver{}, "anthropic") }

// Submit sends one bounded native Messages request through the official SDK.
func (*AnthropicDriver) Submit(ctx context.Context, c *Client, input application.GenerationRequest, key string) (application.GenerationResult, error) {
	if !validateCommonInput(input) || !validKey(key) || input.Capability != "text.generate" || input.Mode != "text_to_text" || input.OutputCount != 1 || len(input.References) != 0 || input.EndpointTag != "" {
		return invalidSubmissionResult(), nil
	}
	limit, ok := integerParam(input.Params["max_tokens"])
	if !ok || limit < 1 || limit > 65536 {
		return invalidSubmissionResult(), nil
	}
	payload := map[string]any{"model": input.ModelID, "messages": []map[string]string{{"role": "user", "content": input.Prompt}}, "stream": false}
	if !applyAllowedParameters(input, payload, false) {
		return invalidSubmissionResult(), nil
	}
	data, status, err := c.request(ctx, input.Config, key, http.MethodPost, "/messages", payload, 4<<20)
	if err != nil {
		return submissionTransportFailure(err), nil
	}
	if status < 200 || status >= 300 {
		return application.GenerationResult{Outcome: application.ProviderOutcomeUnknown, ErrorCode: safeHTTPErrorBody(status, data)}, nil
	}
	var response struct {
		Type    string `json:"type"`
		Stop    string `json:"stop_reason"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			Input  *int64 `json:"input_tokens"`
			Output *int64 `json:"output_tokens"`
			Read   *int64 `json:"cache_read_input_tokens"`
			Write  *int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(data, &response) != nil || response.Type != "message" || response.Stop != "end_turn" || response.Usage.Input == nil || response.Usage.Output == nil {
		return unknownResult(), nil
	}
	u := response.Usage
	if *u.Input < 0 || *u.Output < 0 || *u.Input > 1<<40 || *u.Output > 1<<40 || u.Write != nil && *u.Write != 0 || u.Read != nil && (*u.Read < 0 || *u.Read > 1<<40) {
		return unknownResult(), nil
	}
	var text strings.Builder
	for _, part := range response.Content {
		if part.Type != "text" {
			return unknownResult(), nil
		}
		text.WriteString(part.Text)
	}
	if text.Len() == 0 {
		return unknownResult(), nil
	}
	total := *u.Input
	if u.Read != nil {
		total += *u.Read
	}
	one := int64(1)
	usage, _ := json.Marshal(application.BillableUsage{InputTokens: &total, OutputTokens: u.Output, CachedInputTokens: u.Read, BillableRequests: &one})
	return application.GenerationResult{Outcome: application.ProviderOutcomeCompleted, State: application.ProviderTaskSucceeded, Text: text.String(), Usage: usage}, nil
}

// Query does not resubmit synchronous requests whose result was lost.
func (*AnthropicDriver) Query(context.Context, *Client, catalog.ProviderConfig, string, string) (application.GenerationResult, error) {
	return unknownResult(), nil
}

// Probe only reports availability of the installed protocol; it never generates.
func (*AnthropicDriver) Probe(ctx context.Context, c *Client, config catalog.ProviderConfig, key string) ProbeResult {
	return (&OpenAIStandardStrategy{}).Probe(ctx, c, config, key)
}

// ProbeModelDefaultParams supplies a conservative output bound for explicit tests.
func (*AnthropicDriver) ProbeModelDefaultParams(string) map[string]any {
	return map[string]any{"max_tokens": 64}
}
