package catalog_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/tests/catalogfixture"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestCloudTextProtocolsKeepProviderRoutingIndependent(t *testing.T) {
	for _, preset := range catalogfixture.BuiltinPresets() {
		if !slices.Contains(preset.Config.Capabilities, "text") {
			continue
		}
		t.Run(preset.Slug, func(t *testing.T) {
			calls := 0
			client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if preset.Config.AdapterKey == "openai" {
					require.Equal(t, preset.Config.BaseURL+"/responses", r.URL.String())
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Equal(t, "hello", body["input"])
					require.Equal(t, false, body["store"])
					return providerResponse(200, `{"status":"completed","output":[{"type":"message","status":"completed","content":[{"type":"output_text","text":"fixture reply"}]}],"usage":{"input_tokens":2,"output_tokens":3}}`), nil
				}
				require.Equal(t, preset.Config.BaseURL+"/chat/completions", r.URL.String())
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Contains(t, body, "messages")
				require.NotContains(t, body, "content")
				require.NotContains(t, body, "prompt")
				if preset.Config.AdapterKey != "openrouter" {
					require.NotContains(t, body, "provider")
				}
				return providerResponse(200, `{"choices":[{"message":{"content":"fixture reply"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3}}`), nil
			})})
			result, err := client.Submit(t.Context(), operationapp.GenerationRequest{Config: preset.Config, Capability: "text.generate", Mode: "text_to_text", ModelID: map[bool]string{true: "gpt-6.1-sol", false: "fixture-exact-model"}[preset.Config.AdapterKey == "openai"], Prompt: "hello", OutputCount: 1}, "fixture-key")
			require.NoError(t, err)
			require.Equal(t, operationapp.ProviderOutcome("completed"), result.Outcome)
			require.Equal(t, "fixture reply", result.Text)
			require.Equal(t, 1, calls)
		})
	}
}

func TestArkImageRequestsUseOneImageWithoutOpenRouterFields(t *testing.T) {
	for _, preset := range catalogfixture.BuiltinPresets() {
		if preset.Config.AdapterKey != "volcengine_ark" && preset.Config.AdapterKey != "byteplus" {
			continue
		}
		t.Run(preset.Slug, func(t *testing.T) {
			calls := 0
			client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, preset.Config.BaseURL+"/images/generations", r.URL.String())
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, "b64_json", body["response_format"])
				require.NotContains(t, body, "sequential_image_generation")
				require.Equal(t, "reference image", body["prompt"])
				require.Equal(t, []any{"https://assets.example/allowed.png"}, body["image"])
				require.NotContains(t, body, "provider")
				require.NotContains(t, body, "n")
				require.NotContains(t, body, "content")
				return providerResponse(400, `{}`), nil
			})})
			input := operationapp.GenerationRequest{Config: preset.Config, Capability: "image.generate", Mode: "image2image", ModelID: map[bool]string{true: "gpt-6.1-sol", false: "fixture-exact-model"}[preset.Config.AdapterKey == "openai"], Prompt: "reference image", OutputCount: 1, References: []operationapp.ProviderReference{{Role: "reference_image", URL: "https://assets.example/allowed.png"}}}
			result, err := client.Submit(t.Context(), input, "fixture-key")
			require.NoError(t, err)
			require.Equal(t, operationapp.ProviderOutcome("unknown"), result.Outcome)
			require.Equal(t, 1, calls)
			input.OutputCount = 2
			result, err = client.Submit(t.Context(), input, "fixture-key")
			require.NoError(t, err)
			require.Equal(t, operationapp.ProviderOutcome("not_submitted"), result.Outcome)
			require.Equal(t, 1, calls)
		})
	}
}

func TestCloudDiscoveryDoesNotInventImageOrAccountPermissions(t *testing.T) {
	for _, preset := range catalogfixture.BuiltinPresets() {
		if preset.Config.AdapterKey == "openrouter" || !slices.Contains(preset.Config.Capabilities, "text") {
			continue
		}
		calls := 0
		client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			require.Equal(t, http.MethodGet, r.Method)
			if preset.Config.AdapterKey == "deepseek" {
				require.Equal(t, "/models", r.URL.Path)
				return providerResponse(200, `{"data":[{"id":"account-model"}]}`), nil
			}
			if preset.Config.AdapterKey == "vercel" {
				require.Equal(t, "/v1/credits", r.URL.Path)
				return providerResponse(200, `{"balance":"0.00","total_used":"0.00"}`), nil
			}
			if preset.Config.AdapterKey == "openai" {
				require.Equal(t, "/v1/models", r.URL.Path)
				return providerResponse(200, `{"data":[{"id":"gpt-4o"}]}`), nil
			}
			require.Equal(t, "/api/v3/contents/generations/tasks", r.URL.Path)
			require.Equal(t, "1", r.URL.Query().Get("page_size"))
			return providerResponse(200, `{"items":[]}`), nil
		})})
		result := client.Probe(t.Context(), preset.Config, "fixture-key")
		require.Equal(t, "ok", result.Auth)
		require.Equal(t, 1, calls)
		if preset.Config.AdapterKey == "deepseek" || preset.Config.AdapterKey == "openai" {
			require.Equal(t, "ok", result.Discovery)
			require.Len(t, result.Candidates, 1)
			require.Equal(t, "text.generate", result.Candidates[0].Capability)
			require.JSONEq(t, `{"verified":false}`, string(result.Candidates[0].Metadata))
		} else {
			require.Equal(t, "unsupported", result.Discovery)
			require.Empty(t, result.Candidates)
		}
	}
}

func TestSeedreamProAndFlashOmitUnsupportedGroupAndStreamParameters(t *testing.T) {
	var config domain.ProviderConfig
	for _, preset := range catalogfixture.BuiltinPresets() {
		if preset.Config.AdapterKey == "volcengine_ark" {
			config = preset.Config
			break
		}
	}
	for _, model := range []string{
		"doubao-seedream-5-0-pro-260628",
		"doubao-seedream-5-0-flash-260915",
		"doubao-seedream-5-0-260128",
		"doubao-seedream-4-5-251128",
	} {
		t.Run(model, func(t *testing.T) {
			calls := 0
			client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, model, body["model"])
				require.NotContains(t, body, "sequential_image_generation")
				require.NotContains(t, body, "stream")
				return providerResponse(400, `{}`), nil
			})})
			_, err := client.Submit(t.Context(), operationapp.GenerationRequest{Config: config, Capability: "image.generate", Mode: "text2image", ModelID: model, Prompt: "fixture", OutputCount: 1}, "fixture-key")
			require.NoError(t, err)
			require.Equal(t, 1, calls)
		})
	}
}

func TestTextProtocolsKeepCanonicalCachedTokenEvidence(t *testing.T) {
	for _, adapter := range []string{"deepseek", "volcengine_ark"} {
		for _, raw := range []string{`{"prompt_tokens":1000,"completion_tokens":3,"prompt_cache_hit_tokens":800}`, `{"prompt_tokens":1000,"completion_tokens":3,"prompt_tokens_details":{"cached_tokens":800}}`} {
			var config domain.ProviderConfig
			for _, preset := range catalogfixture.BuiltinPresets() {
				if preset.Config.AdapterKey == adapter {
					config = preset.Config
					break
				}
			}
			client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(_ *http.Request) (*http.Response, error) {
				return providerResponse(200, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":`+raw+`}`), nil
			})})
			result, err := client.Submit(t.Context(), operationapp.GenerationRequest{Config: config, ModelID: "fixture", Capability: "text.generate", Mode: "text_to_text", Prompt: "hello", OutputCount: 1}, "fixture-key")
			require.NoError(t, err)
			require.Equal(t, operationapp.ProviderOutcome("completed"), result.Outcome)
			var usage map[string]int64
			require.NoError(t, json.Unmarshal(result.Usage, &usage))
			require.Equal(t, int64(800), usage["cached_input_tokens"])
		}
	}
}
