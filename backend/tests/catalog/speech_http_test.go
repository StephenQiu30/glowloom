package catalog_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func speechConfig() domain.ProviderConfig {
	return domain.ProviderConfig{AdapterKey: "volcengine_speech", BaseURL: "https://openspeech.bytedance.com/api/v3", Region: domain.RegionDomestic, Capabilities: []string{"audio"}, TimeoutSeconds: 120, ConcurrencyLimit: 2, RateLimitPerMin: 30}
}
func TestDoubaoTTSUsesResourceHeadersAndCollectsAudioChunks(t *testing.T) {
	audio := append([]byte("ID3"), bytes.Repeat([]byte{0}, 100)...)
	calls := 0
	client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "/api/v3/tts/unidirectional", r.URL.Path)
		require.Equal(t, "fixture-speech-key", r.Header.Get("X-Api-Key"))
		require.Equal(t, "seed-tts-2.0", r.Header.Get("X-Api-Resource-Id"))
		require.NotEmpty(t, r.Header.Get("X-Api-Request-Id"))
		require.Empty(t, r.Header.Get("Authorization"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		params := body["req_params"].(map[string]any)
		require.Equal(t, "你好", params["text"])
		require.Equal(t, "zh_female_vv_uranus_bigtts", params["speaker"])
		return providerResponse(200, `{"code":0,"data":"`+base64.StdEncoding.EncodeToString(audio[:45])+`"}`+"\n"+`{"code":0,"data":"`+base64.StdEncoding.EncodeToString(audio[45:])+`","usage":{"text_words":2}}`+"\n"+`{"code":20000000}`), nil
	})})
	result, err := client.Submit(t.Context(), application.GenerationRequest{Config: speechConfig(), ModelID: "seed-tts-2.0", Capability: "audio.generate", Mode: "text2audio", Prompt: "你好", OutputCount: 1, Params: map[string]any{"speaker": "zh_female_vv_uranus_bigtts"}}, "fixture-speech-key")
	require.NoError(t, err)
	require.Equal(t, "completed", result.Outcome)
	require.Len(t, result.Audio, 1)
	require.Equal(t, "audio/mpeg", result.Audio[0].MediaType)
	require.Equal(t, audio, result.Audio[0].Bytes)
	require.Equal(t, 1, calls)
}
func TestDoubaoTTSRejectsWrongModelsAndNeverRetriesUnknown(t *testing.T) {
	calls := 0
	client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(io.MultiReader(strings.NewReader(`{"code":0,"data":"SUQz"}`), brokenSpeechReader{})), Header: make(http.Header)}, nil
	})})
	input := application.GenerationRequest{Config: speechConfig(), ModelID: "seed-tts-2.0", Capability: "audio.generate", Mode: "text2audio", Prompt: "验收", OutputCount: 1, Params: map[string]any{"speaker": "zh_female_vv_uranus_bigtts"}}
	result, err := client.Submit(t.Context(), input, "fixture-speech-key")
	require.NoError(t, err)
	require.Equal(t, "unknown", result.Outcome)
	require.Equal(t, 1, calls)
	input.ModelID = "doubao-seed-2-1-pro-260915"
	result, err = client.Submit(t.Context(), input, "fixture-speech-key")
	require.NoError(t, err)
	require.Equal(t, "not_submitted", result.Outcome)
	require.Equal(t, 1, calls)
}

type brokenSpeechReader struct{}

func (brokenSpeechReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestDoubaoSpeechMetadataProbeCannotGeneratePaidAudio(t *testing.T) {
	calls := 0
	client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(*http.Request) (*http.Response, error) { calls++; return providerResponse(200, `{}`), nil })})
	probe := client.Probe(t.Context(), speechConfig(), "fixture-key")
	require.Equal(t, "unknown", probe.Auth)
	require.Equal(t, "unsupported", probe.Discovery)
	require.Zero(t, calls)
}

func TestDoubaoTTSRejectsMalformedResponsesAndAuthenticatedRedirects(t *testing.T) {
	for _, body := range []string{`{}`, `{"code":0,"data":"not base64"}`, `{"code":0,"data":"e30="}`, `{"code":0,"data":"SUQz"} broken`} {
		t.Run(body, func(t *testing.T) {
			calls := 0
			client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(*http.Request) (*http.Response, error) { calls++; return providerResponse(200, body), nil })})
			result, err := client.Submit(t.Context(), application.GenerationRequest{Config: speechConfig(), ModelID: "seed-tts-2.0", Capability: "audio.generate", Mode: "text2audio", Prompt: "验收", OutputCount: 1}, "fixture-key")
			require.NoError(t, err)
			require.Equal(t, "unknown", result.Outcome)
			require.Len(t, result.Audio, 0)
			require.Equal(t, 1, calls)
		})
	}
	calls := 0
	client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(*http.Request) (*http.Response, error) {
		calls++
		response := providerResponse(307, `{}`)
		response.Header.Set("Location", "https://other.example/steal")
		return response, nil
	})})
	result, err := client.Submit(t.Context(), application.GenerationRequest{Config: speechConfig(), ModelID: "seed-tts-2.0", Capability: "audio.generate", Mode: "text2audio", Prompt: "验收", OutputCount: 1}, "fixture-key")
	require.NoError(t, err)
	require.Equal(t, "unknown", result.Outcome)
	require.Equal(t, 1, calls)
}

func TestSeedance25FirstFrameAlwaysUsesAdaptiveRatio(t *testing.T) {
	for _, mode := range []string{"text2video", "image2video"} {
		t.Run(mode, func(t *testing.T) {
			config := domain.BuiltinPresets()[0].Config
			client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.EqualValues(t, 30, body["duration"])
				ratio := "16:9"
				if mode == "image2video" {
					ratio = "adaptive"
				}
				require.Equal(t, ratio, body["ratio"])
				return providerResponse(200, `{"id":"fixture-video"}`), nil
			})})
			input := application.GenerationRequest{Config: config, ModelID: "doubao-seedance-2-5-260628", Capability: "video.generate", Mode: mode, Prompt: "场景", OutputCount: 1, Params: map[string]any{"duration_ms": 30000, "aspect_ratio": "16:9"}}
			if mode == "image2video" {
				input.References = []application.ProviderReference{{Role: "first_frame", URL: "https://example.invalid/reference.png"}}
			}
			result, err := client.Submit(t.Context(), input, "fixture-key")
			require.NoError(t, err)
			require.Equal(t, "accepted", result.Outcome)
		})
	}
}

func TestDoubaoTTSStrictStreamRequiresAnExplicitCompleteTerminal(t *testing.T) {
	audio := []byte("ID3 controlled speech protocol fixture")
	frame := `data: {"code":0,"data":"` + base64.StdEncoding.EncodeToString(audio) + `","usage":{"text_words":2}}` + "\n\n"
	for _, test := range []struct {
		name, body, contentType, outcome string
	}{
		{"complete", frame + "data: {\"code\":20000000}\n\n", "text/event-stream", "completed"},
		{"missing terminal", frame, "text/event-stream", "unknown"},
		{"truncated event", frame + "data: {\"code\":20000000}\n", "text/event-stream", "unknown"},
		{"wrong protocol", frame, "application/json", "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			requestID := "c548d094-cd8f-4e44-932f-faf50d92f405"
			client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "/api/v3/tts/unidirectional/sse", r.URL.Path)
				require.Equal(t, requestID, r.Header.Get("X-Api-Request-Id"))
				response := providerResponse(200, test.body)
				response.Header.Set("Content-Type", test.contentType)
				response.Header.Set("X-Tt-Logid", "controlled-log")
				return response, nil
			})})
			result, err := client.Submit(t.Context(), application.GenerationRequest{StrictSpeechTerminal: true, ProviderRequestID: requestID, Config: speechConfig(), ModelID: "seed-tts-2.0", Capability: "audio.generate", Mode: "text2audio", Prompt: "你好", OutputCount: 1}, "fixture-key")
			require.NoError(t, err)
			require.Equal(t, test.outcome, result.Outcome)
			require.Equal(t, 1, calls)
			if test.outcome == "completed" {
				require.Equal(t, 20000000, result.ProviderTerminalCode)
				require.Equal(t, "controlled-log", result.ProviderLogID)
				require.Equal(t, audio, result.Audio[0].Bytes)
			} else {
				require.Empty(t, result.Audio)
			}
		})
	}
}

func TestSeedance25OmniUsesTypedReferenceObjects(t *testing.T) {
	for _, kind := range []string{"video", "audio"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				var payload map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
				content := payload["content"].([]any)
				ref := content[1].(map[string]any)
				require.Equal(t, kind+"_url", ref["type"])
				require.Equal(t, "reference_"+kind, ref["role"])
				require.Equal(t, "https://media.example/input", ref[kind+"_url"].(map[string]any)["url"])
				return providerResponse(200, `{"id":"controlled-seedance-task"}`), nil
			})})
			var config domain.ProviderConfig
			for _, preset := range domain.BuiltinPresets() {
				if preset.Config.AdapterKey == "volcengine_ark" {
					config = preset.Config
				}
			}
			result, err := client.Submit(t.Context(), application.GenerationRequest{Config: config, ModelID: "doubao-seedance-2-5-260628", Capability: "video.generate", Mode: "omni_reference", Prompt: "测试", OutputCount: 1, References: []application.ProviderReference{{Role: "reference_" + kind, MediaKind: kind, URL: "https://media.example/input"}}}, "fixture-key")
			require.NoError(t, err)
			require.Equal(t, "accepted", result.Outcome)
			require.Equal(t, 1, calls)
		})
	}
}
