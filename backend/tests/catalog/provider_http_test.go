package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

type providerTransport func(*http.Request) (*http.Response, error)

func (f providerTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func providerResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func TestProviderHTTPRoutesAndDoesNotRetryUnknownSubmission(t *testing.T) {
	for _, capability := range []string{"text.generate", "image.generate", "video.generate"} {
		t.Run(capability, func(t *testing.T) {
			calls := 0
			client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
				require.Equal(t, "openrouter.ai", r.URL.Host)
				paths := map[string]string{"text.generate": "/api/v1/chat/completions", "image.generate": "/api/v1/images", "video.generate": "/api/v1/videos"}
				require.Equal(t, paths[capability], r.URL.Path)
				var payload map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
				require.Equal(t, "test/model", payload["model"])
				return nil, errors.New("transport interrupted with secret fixture-key")
			})})
			result, err := client.Submit(context.Background(), operationapp.GenerationRequest{Config: domain.BuiltinPresets()[1].Config, ModelID: "test/model", Capability: capability, Mode: map[string]string{"text.generate": "text_to_text", "image.generate": "text2image", "video.generate": "text2video"}[capability], Prompt: "test", OutputCount: 1}, "fixture-key")
			require.NoError(t, err)
			require.Equal(t, "unknown", result.Outcome)
			require.Equal(t, 1, calls)
			require.NotContains(t, result.ErrorCode, "fixture-key")
		})
	}
}
func TestProviderHTTPRejectsUnknownParametersBeforeNetwork(t *testing.T) {
	client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected network request"); return nil, nil })})
	result, err := client.Submit(t.Context(), operationapp.GenerationRequest{Config: domain.BuiltinPresets()[1].Config, ModelID: "test/model", Capability: "image.generate", Mode: "text2image", Prompt: "test", OutputCount: 1, Params: map[string]any{"callback_url": "https://evil.test"}}, "key")
	require.NoError(t, err)
	require.Equal(t, "not_submitted", result.Outcome)
}
func TestProviderHTTPRejectsForeignAuthenticatedVideoURL(t *testing.T) {
	client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(_ *http.Request) (*http.Response, error) {
		return providerResponse(200, `{"id":"task-1","status":"completed","unsigned_urls":["https://evil.test/video"],"usage":{"cost":0.25}}`), nil
	})})
	result, err := client.Query(t.Context(), domain.BuiltinPresets()[1].Config, "task-1", "key")
	require.NoError(t, err)
	require.Equal(t, "unknown", result.State)
	require.Empty(t, result.URLs)
}

func TestProviderModesRejectMismatchedReferencesBeforeDispatch(t *testing.T) {
	client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(*http.Request) (*http.Response, error) { t.Fatal("invalid mode dispatched"); return nil, nil })})
	for _, input := range []operationapp.GenerationRequest{
		{Capability: "image.generate", Mode: "text2image", References: []operationapp.ProviderReference{{Role: "reference_image", URL: "https://assets.example/image.png"}}},
		{Capability: "video.generate", Mode: "image2video"},
		{Capability: "video.generate", Mode: "text2video", References: []operationapp.ProviderReference{{Role: "first_frame", URL: "https://assets.example/image.png"}}},
		{Capability: "text.generate", Mode: "invented"},
	} {
		input.Config = domain.BuiltinPresets()[1].Config
		input.ModelID = "fixture/model"
		input.Prompt = "test"
		input.OutputCount = 1
		result, err := client.Submit(t.Context(), input, "fixture-key")
		require.NoError(t, err)
		require.Equal(t, "not_submitted", result.Outcome)
	}
}

func TestProviderVideoMapsInternalDurationWithoutChangingFrozenInput(t *testing.T) {
	for _, config := range domain.BuiltinPresets() {
		calls := 0
		client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, float64(5), body["duration"])
			require.NotContains(t, body, "duration_ms")
			if config.Config.AdapterKey == "volcengine_ark" {
				require.Equal(t, "16:9", body["ratio"])
			}
			return providerResponse(200, `{"id":"fixture-task"}`), nil
		})})
		params := map[string]any{"duration_ms": int64(5000), "aspect_ratio": "16:9"}
		result, err := client.Submit(t.Context(), operationapp.GenerationRequest{Config: config.Config, ModelID: "fixture", Capability: "video.generate", Mode: "text2video", Prompt: "fixture", OutputCount: 1, Params: params}, "fixture-only")
		require.NoError(t, err)
		require.Equal(t, "accepted", result.Outcome)
		require.Equal(t, 1, calls)
		require.Equal(t, int64(5000), params["duration_ms"])
		params["duration"] = 5
		result, err = client.Submit(t.Context(), operationapp.GenerationRequest{Config: config.Config, ModelID: "fixture", Capability: "video.generate", Mode: "text2video", Prompt: "fixture", OutputCount: 1, Params: params}, "fixture-only")
		require.NoError(t, err)
		require.Equal(t, "not_submitted", result.Outcome)
		require.Equal(t, 1, calls)
	}
}
