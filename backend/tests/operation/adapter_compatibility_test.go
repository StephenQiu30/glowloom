package operation_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/tests/catalogfixture"

	"github.com/stretchr/testify/require"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

type interruptedAdapterBody struct{}

func (*interruptedAdapterBody) Read(buffer []byte) (int, error) {
	return copy(buffer, `{"id":"accepted"`), io.ErrUnexpectedEOF
}
func (*interruptedAdapterBody) Close() error { return nil }

type adapterTransport func(*http.Request) (*http.Response, error)

func (f adapterTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func adapterResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestAdapterCompatibilityRegistryRejectsDuplicateProtocols(t *testing.T) {
	driver, ok := providerhttp.GetDriver("openai")
	require.True(t, ok)
	require.Panics(t, func() { providerhttp.RegisterDriver(driver, "openai") })
	require.Panics(t, func() { providerhttp.RegisterDriver(nil, "empty-driver") })
	require.Panics(t, func() { providerhttp.RegisterDriver(driver, "") })
}

func TestAdapterCompatibilityInstalledTextProtocols(t *testing.T) {
	for _, preset := range catalogfixture.BuiltinPresets() {
		if !strings.Contains(strings.Join(preset.Config.Capabilities, ","), "text") {
			continue
		}
		t.Run(preset.Config.AdapterKey, func(t *testing.T) {
			for _, status := range []int{200, 401, 402, 429} {
				t.Run(http.StatusText(status), func(t *testing.T) {
					calls := 0
					client := providerhttp.NewClient(&http.Client{Transport: adapterTransport(func(r *http.Request) (*http.Response, error) {
						calls++
						require.Equal(t, "POST", r.Method)
						require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
						require.Empty(t, r.Header.Get("Idempotency-Key"))
						var body map[string]any
						require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
						require.Equal(t, "gpt-6.1-sol", body["model"])
						if preset.Config.AdapterKey == "openai" {
							require.Equal(t, preset.Config.BaseURL+"/responses", r.URL.String())
							require.Equal(t, "hello", body["input"])
						} else {
							require.Equal(t, preset.Config.BaseURL+"/chat/completions", r.URL.String())
							require.Contains(t, body, "messages")
						}
						if preset.Config.AdapterKey == "openrouter" {
							require.Equal(t, "https://glowloom.com", r.Header.Get("HTTP-Referer"))
						} else {
							require.Empty(t, r.Header.Get("HTTP-Referer"))
						}
						if status != 200 {
							return adapterResponse(status, `{"error":{"message":"fixture-key must never echo"}}`), nil
						}
						if preset.Config.AdapterKey == "openai" {
							return adapterResponse(200, `{"status":"completed","output":[{"type":"message","status":"completed","content":[{"type":"output_text","text":"answer"}]}]}`), nil
						}
						return adapterResponse(200, `{"choices":[{"message":{"content":"answer"},"finish_reason":"stop"}]}`), nil
					})})
					result, err := client.Submit(t.Context(), application.GenerationRequest{Config: preset.Config, Capability: "text.generate", Mode: "text_to_text", ModelID: "gpt-6.1-sol", Prompt: "hello", OutputCount: 1}, "fixture-key")
					require.NoError(t, err)
					require.Equal(t, 1, calls)
					if status == 200 {
						require.EqualValues(t, "completed", result.Outcome)
						require.Equal(t, "answer", result.Text)
					} else {
						require.NotEqual(t, "completed", result.Outcome)
						require.NotEqual(t, "not_submitted", result.Outcome)
						require.NotContains(t, result.ErrorCode, "fixture-key")
					}
				})
			}
		})
	}
}

func TestAdapterUnknownNeverResubmits(t *testing.T) {
	config := catalogfixture.BuiltinPresets()[0].Config
	posts, gets := 0, 0
	client := providerhttp.NewClient(&http.Client{Transport: adapterTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost {
			posts++
			return adapterResponse(200, `{"id":"original-task"}`), nil
		}
		gets++
		require.Equal(t, config.BaseURL+"/contents/generations/tasks/original-task", r.URL.String())
		return adapterResponse(200, `{"id":"original-task","status":"running"}`), nil
	})})
	result, err := client.Submit(t.Context(), application.GenerationRequest{Config: config, Capability: "video.generate", Mode: "text2video", ModelID: "doubao-seedance-1-0-pro-250528", Prompt: "hello", OutputCount: 1}, "fixture-key")
	require.NoError(t, err)
	require.Equal(t, "original-task", result.TaskID)
	for range 3 {
		_, err = client.Query(t.Context(), config, result.TaskID, "rotated-fixture-key")
		require.NoError(t, err)
	}
	require.Equal(t, 1, posts)
	require.Equal(t, 3, gets)
	// A truncated accepted response stays unknown and is never retried by transport.
	brokenPosts := 0
	broken := providerhttp.NewClient(&http.Client{Transport: adapterTransport(func(*http.Request) (*http.Response, error) {
		brokenPosts++
		response := adapterResponse(200, "")
		response.Body = &interruptedAdapterBody{}
		return response, nil
	})})
	unknown, err := broken.Submit(t.Context(), application.GenerationRequest{Config: config, Capability: "video.generate", Mode: "text2video", ModelID: "doubao-seedance-1-0-pro-250528", Prompt: "hello", OutputCount: 1}, "fixture-key")
	require.NoError(t, err)
	require.EqualValues(t, "unknown", unknown.Outcome)
	require.Equal(t, 1, brokenPosts)
}

func TestBytePlusAPImagePresetUsesExistingArkProtocol(t *testing.T) {
	var config catalog.ProviderConfig
	for _, p := range catalogfixture.BuiltinPresets() {
		if p.Config.AdapterKey == "byteplus" {
			config = p.Config
		}
	}
	for _, candidate := range catalogfixture.BuiltinModelCatalog("byteplus").Models {
		if candidate.RequiresModelSelection {
			continue
		}
		t.Run(candidate.Key, func(t *testing.T) {
			calls := 0
			client := providerhttp.NewClient(&http.Client{Transport: adapterTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "https://ark.ap-southeast.bytepluses.com/api/v3/images/generations", r.URL.String())
				require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, candidate.ProviderModelID, body["model"])
				require.Equal(t, "1.5K", body["size"])
				require.NotContains(t, body, "sequential_image_generation")
				return adapterResponse(400, `{"error":{"code":"fixture_rejection"}}`), nil
			})})
			_, err := client.Submit(t.Context(), application.GenerationRequest{Config: config, ModelID: candidate.ProviderModelID, Capability: "image.generate", Mode: "text2image", Prompt: "cloud", OutputCount: 1, Params: map[string]any{"size": "1.5K"}}, "fixture-key")
			require.NoError(t, err)
			require.Equal(t, 1, calls, "fixture transport never reaches a supplier")
		})
	}
}
