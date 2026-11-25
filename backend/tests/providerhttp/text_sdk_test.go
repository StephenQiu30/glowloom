package providerhttp_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/execution/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/adapter/providerhttp"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/application"
)

func TestNativeSDKRequestsStaySingleAndBounded(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "unapproved-environment-key")
	t.Setenv("OPENAI_ORG_ID", "unapproved-org")
	t.Setenv("OPENAI_PROJECT_ID", "unapproved-project")
	t.Setenv("ANTHROPIC_CUSTOM_HEADERS", "X-Unapproved: secret")
	t.Setenv("ANTHROPIC_BASE_URL", "https://unapproved.example")

	for _, protocol := range []string{"chat", "responses", "anthropic"} {
		t.Run(protocol, func(t *testing.T) {
			adapter, path, base := "openai", "/chat/completions", "https://api.openai.com/v1"
			if protocol == "responses" {
				path = "/responses"
			}
			if protocol == "anthropic" {
				adapter, path, base = "anthropic", "/messages", "https://api.anthropic.com/v1"
			}
			config := catalog.ProviderConfig{AdapterKey: adapter, BaseURL: base, Region: "overseas", Capabilities: []string{"text"}, TimeoutSeconds: 5, ConcurrencyLimit: 1, RateLimitPerMin: 10}
			for _, status := range []int{200, 302, 429, 500} {
				calls := 0
				c := providerhttp.NewClient(&http.Client{Transport: gatewayTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					require.Empty(t, r.Header.Get("OpenAI-Organization"))
					require.Empty(t, r.Header.Get("OpenAI-Project"))
					require.Empty(t, r.Header.Get("X-Unapproved"))
					require.Equal(t, "/v1"+path, r.URL.Path)
					if adapter == "anthropic" {
						require.Equal(t, "fixture", r.Header.Get("X-Api-Key"))
						require.NotEmpty(t, r.Header.Get("Anthropic-Version"))
					} else {
						require.Equal(t, "Bearer fixture", r.Header.Get("Authorization"))
					}
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					require.Contains(t, string(body), `"reviewed_extension":true`)
					return &http.Response{StatusCode: status, Header: http.Header{"X-Request-Id": []string{"req_fixture"}, "Location": []string{"https://other.example/leak"}}, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
				})})
				data, got, id, err := c.TextRequest(t.Context(), config, "fixture", path, json.RawMessage(`{"model":"verified","reviewed_extension":true}`), 1024, nil)
				require.NoError(t, err)
				require.Equal(t, status, got)
				require.Equal(t, "req_fixture", id)
				require.JSONEq(t, `{"ok":true}`, string(data))
				require.Equal(t, 1, calls)
			}
			c := providerhttp.NewClient(&http.Client{Transport: gatewayTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 500, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", 2048)))}, nil
			})})
			_, _, _, err := c.TextRequest(t.Context(), config, "fixture", path, map[string]string{"model": "verified"}, 16, nil)
			require.Error(t, err)
		})
	}
}

func TestAnthropicNativeTextPreservesUsageBuckets(t *testing.T) {
	config := catalog.ProviderConfig{AdapterKey: "anthropic", BaseURL: "https://api.anthropic.com/v1", Region: "overseas", Capabilities: []string{"text"}, TimeoutSeconds: 5, ConcurrencyLimit: 1, RateLimitPerMin: 10}
	for _, scenario := range []string{"success", "missing_usage", "cache_write", "unknown_block", "rate_limit", "unsupported_parameter"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			client := providerhttp.NewClient(&http.Client{Transport: gatewayTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "/v1/messages", r.URL.Path)
				var request map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				require.EqualValues(t, 32, request["max_tokens"])
				body := `{"type":"message","stop_reason":"end_turn","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":7,"output_tokens":3,"cache_read_input_tokens":2,"cache_creation_input_tokens":0}}`
				status := 200
				switch scenario {
				case "missing_usage":
					body = strings.ReplaceAll(body, `"input_tokens":7,`, "")
				case "cache_write":
					body = strings.ReplaceAll(body, `"cache_creation_input_tokens":0`, `"cache_creation_input_tokens":9`)
				case "unknown_block":
					body = strings.ReplaceAll(body, `"type":"text"`, `"type":"thinking"`)
				case "rate_limit":
					status = 429
				}
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})})
			params := map[string]any{"max_tokens": 32}
			if scenario == "unsupported_parameter" {
				params["thinking"] = true
			}
			result, err := client.Submit(t.Context(), application.GenerationRequest{Config: config, ModelID: "verified-model", Capability: "text.generate", Mode: "text_to_text", OutputCount: 1, Prompt: "fixture", Params: params}, "fixture")
			require.NoError(t, err)
			if scenario == "success" {
				require.Equal(t, application.ProviderOutcomeCompleted, result.Outcome)
				usage, err := application.ParseBillableUsage(result.Usage)
				require.NoError(t, err)
				require.EqualValues(t, 9, *usage.InputTokens)
				require.EqualValues(t, 2, *usage.CachedInputTokens)
				require.EqualValues(t, 3, *usage.OutputTokens)
			} else {
				require.NotEqual(t, application.ProviderOutcomeCompleted, result.Outcome)
			}
			if scenario == "unsupported_parameter" {
				require.Zero(t, calls)
			} else {
				require.Equal(t, 1, calls)
			}
		})
	}
}

func TestAnthropicDiscoveryUsesNativeHeadersWithoutGenerating(t *testing.T) {
	config := catalog.ProviderConfig{AdapterKey: "anthropic", BaseURL: "https://api.anthropic.com/v1", Region: "overseas", Capabilities: []string{"text"}, TimeoutSeconds: 5, ConcurrencyLimit: 1, RateLimitPerMin: 10}
	calls := 0
	client := providerhttp.NewClient(&http.Client{Transport: gatewayTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v1/models", r.URL.Path)
		require.Equal(t, "fixture", r.Header.Get("X-Api-Key"))
		require.Equal(t, "2023-06-01", r.Header.Get("Anthropic-Version"))
		require.Empty(t, r.Header.Get("Authorization"))
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"fixture-model"}]}`))}, nil
	})})
	result := client.Probe(t.Context(), config, "fixture")
	require.Equal(t, "ok", result.Auth)
	require.Len(t, result.Candidates, 1)
	require.Equal(t, 1, calls)
}
