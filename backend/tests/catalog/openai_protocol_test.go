package catalog_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/tests/catalogfixture"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func openAIConfig(t *testing.T) domain.ProviderConfig {
	t.Helper()
	for _, p := range catalogfixture.BuiltinPresets() {
		if p.Config.AdapterKey == "openai" {
			return p.Config
		}
	}
	t.Fatal("missing preset")
	return domain.ProviderConfig{}
}

func TestOpenAIResponsesProtocol(t *testing.T) {
	for _, tc := range []struct{ name, response, outcome string }{
		{"completed", `{"status":"completed","output":[{"type":"reasoning"},{"type":"message","status":"completed","content":[{"type":"output_text","text":"fixture reply"}]}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`, "completed"},
		{"incomplete", `{"status":"incomplete","output":[{"type":"message","content":[{"type":"output_text","text":"partial"}]}]}`, "unknown"},
		{"refused", `{"status":"completed","output":[{"type":"message","content":[{"type":"refusal","refusal":"no"}]}]}`, "unknown"},
		{"empty", `{"status":"completed","output":[]}`, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "https://api.openai.com/v1/responses", r.URL.String())
				require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, "hello", body["input"])
				require.Equal(t, false, body["store"])
				require.Equal(t, float64(128), body["max_output_tokens"])
				require.NotContains(t, body, "max_tokens")
				require.NotContains(t, body, "messages")
				return providerResponse(200, tc.response), nil
			})})
			result, err := client.Submit(t.Context(), operationapp.GenerationRequest{Config: openAIConfig(t), Capability: "text.generate", Mode: "text_to_text", ModelID: "gpt-6.1-sol", Prompt: "hello", OutputCount: 1, Params: map[string]any{"max_tokens": 128}}, "fixture-key")
			require.NoError(t, err)
			require.Equal(t, operationapp.ProviderOutcome(tc.outcome), result.Outcome)
			require.Equal(t, 1, calls)
			if tc.outcome == "completed" {
				require.Equal(t, "fixture reply", result.Text)
				require.JSONEq(t, `{"input_tokens":2,"output_tokens":3,"total_tokens":5}`, string(result.Usage))
			}
		})
	}
}

func TestOpenAICandidatesCanCreateConservativeVersions(t *testing.T) {
	catalog := catalogfixture.BuiltinModelCatalog("openai")
	require.NotEmpty(t, catalog.Models)
	for _, candidate := range catalog.Models {
		require.NotContains(t, []string{"o1", "o1-mini", "o3-mini", "dall-e-3", "gpt-4-turbo"}, candidate.ProviderModelID)
		_, err := candidate.OnboardingVersion(uuid.New(), uuid.New())
		require.NoError(t, err, candidate.Key)
	}
}

func TestOpenAIImageOmitsRetiredDalleParameters(t *testing.T) {
	calls := 0
	client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "/v1/images/generations", r.URL.Path)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.NotContains(t, body, "response_format")
		require.NotContains(t, body, "seed")
		require.NotContains(t, body, "watermark")
		require.Equal(t, "1024x1024", body["size"])
		require.Equal(t, "medium", body["quality"])
		return providerResponse(200, `{"data":[{"b64_json":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aZ1kAAAAASUVORK5CYII="}]}`), nil
	})})
	input := operationapp.GenerationRequest{Config: openAIConfig(t), Capability: "image.generate", Mode: "text2image", ModelID: "gpt-image-2.5-sunburst", Prompt: "hello", OutputCount: 1, Params: map[string]any{"size": "1024x1024", "quality": "medium"}}
	result, err := client.Submit(t.Context(), input, "fixture-key")
	require.NoError(t, err)
	require.Equal(t, operationapp.ProviderOutcome("completed"), result.Outcome)
	require.Len(t, result.Images, 1)
	input.Params["seed"] = 1
	result, err = client.Submit(t.Context(), input, "fixture-key")
	require.NoError(t, err)
	require.Equal(t, operationapp.ProviderOutcome("not_submitted"), result.Outcome)
	require.Equal(t, 1, calls)
}

func TestOpenAILegacyFrozenTextKeepsChatProtocolAndParameters(t *testing.T) {
	client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, 0.5, body["temperature"])
		return providerResponse(200, `{"choices":[{"message":{"content":"legacy"},"finish_reason":"stop"}]}`), nil
	})})
	result, err := client.Submit(t.Context(), operationapp.GenerationRequest{Config: openAIConfig(t), Capability: "text.generate", Mode: "text_to_text", ModelID: "gpt-4o", Prompt: "hello", OutputCount: 1, Params: map[string]any{"temperature": 0.5}}, "fixture-key")
	require.NoError(t, err)
	require.Equal(t, operationapp.ProviderOutcome("completed"), result.Outcome)
}

func TestOpenAIImageAliasDoesNotUseDalleResponseFormat(t *testing.T) {
	client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.NotContains(t, body, "response_format")
		return providerResponse(200, `{"data":[]}`), nil
	})})
	_, err := client.Submit(t.Context(), operationapp.GenerationRequest{Config: openAIConfig(t), Capability: "image.generate", Mode: "text2image", ModelID: "chatgpt-image-latest", Prompt: "hello", OutputCount: 1}, "fixture-key")
	require.NoError(t, err)
}
