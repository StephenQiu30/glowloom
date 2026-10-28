package catalog_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestCloudTextProtocolsKeepProviderRoutingIndependent(t *testing.T) {
	for _, preset := range domain.BuiltinPresets() {
		if preset.Config.AdapterKey == "volcengine_speech" {
			continue
		}
		t.Run(preset.Slug, func(t *testing.T) {
			calls := 0
			client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
				calls++
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
			result, err := client.Submit(t.Context(), operationapp.GenerationRequest{Config: preset.Config, Capability: "text.generate", Mode: "text_to_text", ModelID: "fixture-exact-model", Prompt: "hello", OutputCount: 1}, "fixture-key")
			require.NoError(t, err)
			require.Equal(t, "completed", result.Outcome)
			require.Equal(t, "fixture reply", result.Text)
			require.Equal(t, 1, calls)
		})
	}
}

func TestArkImageRequestsUseSingleImageContractWithoutOpenRouterFields(t *testing.T) {
	for _, preset := range domain.BuiltinPresets() {
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
				require.Equal(t, "disabled", body["sequential_image_generation"])
				require.Equal(t, "reference image", body["prompt"])
				require.Equal(t, []any{"https://assets.example/allowed.png"}, body["image"])
				require.NotContains(t, body, "provider")
				require.NotContains(t, body, "n")
				require.NotContains(t, body, "content")
				return providerResponse(400, `{}`), nil
			})})
			input := operationapp.GenerationRequest{Config: preset.Config, Capability: "image.generate", Mode: "image2image", ModelID: "fixture-exact-model", Prompt: "reference image", OutputCount: 1, References: []operationapp.ProviderReference{{Role: "reference_image", URL: "https://assets.example/allowed.png"}}}
			result, err := client.Submit(t.Context(), input, "fixture-key")
			require.NoError(t, err)
			require.Equal(t, "rejected", result.Outcome)
			require.Equal(t, 1, calls)
			input.OutputCount = 2
			result, err = client.Submit(t.Context(), input, "fixture-key")
			require.NoError(t, err)
			require.Equal(t, "not_submitted", result.Outcome)
			require.Equal(t, 1, calls)
		})
	}
}

func TestCloudDiscoveryDoesNotInventImageOrAccountPermissions(t *testing.T) {
	for _, preset := range domain.BuiltinPresets() {
		if preset.Config.AdapterKey == "openrouter" || preset.Config.AdapterKey == "volcengine_speech" {
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
			require.Equal(t, "/api/v3/contents/generations/tasks", r.URL.Path)
			require.Equal(t, "1", r.URL.Query().Get("page_size"))
			return providerResponse(200, `{"items":[]}`), nil
		})})
		result := client.Probe(t.Context(), preset.Config, "fixture-key")
		require.Equal(t, "ok", result.Auth)
		require.Equal(t, 1, calls)
		if preset.Config.AdapterKey == "deepseek" {
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
