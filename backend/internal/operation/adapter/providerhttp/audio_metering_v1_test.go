package providerhttp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestOmniAudioPreservesOriginalDecimalEvidence(t *testing.T) {
	audio := base64.StdEncoding.EncodeToString(append([]byte("ID3"), bytes.Repeat([]byte{0}, 100)...))
	for _, tc := range []struct {
		duration, original string
		ok                 bool
	}{{"30", "60.0000000000000000001", true}, {"30", "120.0000000000000000001", false}, {"30", "60,\"original_duration\":30", false}} {
		calls := 0
		c := NewClient(&http.Client{Transport: creationTransport(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"audio":"` + audio + `","duration":` + tc.duration + `,"original_duration":` + tc.original + `}`))}, nil
		})})
		in := omniAudioInput()
		in.Prompt = "海浪声"
		got, err := c.Submit(t.Context(), in, "fixture-key")
		require.NoError(t, err)
		require.Equal(t, 1, calls)
		if tc.ok {
			require.Equal(t, application.ProviderOutcomeCompleted, got.Outcome)
			require.Contains(t, string(got.Usage), `"original_duration":`+tc.original)
		} else {
			require.Equal(t, application.ProviderOutcomeUnknown, got.Outcome)
		}
	}
}

func TestStrictTTSRequiresUsageInCompleteTerminal(t *testing.T) {
	audio := base64.StdEncoding.EncodeToString(append([]byte("ID3"), bytes.Repeat([]byte{0}, 100)...))
	for _, terminal := range []string{`{"code":20000000}`, `{"code":20000000,"usage":{"text_words":null}}`, `{"code":20000000,"usage":{"text_words":0}}`, `{"code":20000000,"usage":{"text_words":1,"text_words":2}}`} {
		calls := 0
		c := NewClient(&http.Client{Transport: creationTransport(func(*http.Request) (*http.Response, error) {
			calls++
			body := "data: {\"code\":0,\"data\":\"" + audio + "\",\"usage\":{\"text_words\":8}}\n\ndata: " + terminal + "\n\n"
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
		})})
		in := application.GenerationRequest{Config: creationConfig("volcengine_speech"), ModelID: "seed-tts-2.0", Capability: "audio.generate", Mode: "text2audio", Prompt: "你 好\n", OutputCount: 1, StrictSpeechTerminal: true, RequireSpeechTerminalUsage: true}
		got, err := c.Submit(t.Context(), in, "fixture-key")
		require.NoError(t, err)
		require.Equal(t, application.ProviderOutcomeUnknown, got.Outcome, terminal)
		require.Equal(t, 1, calls)
	}
}

func TestCharacterContractAssemblesAudioAndUsesTerminalUsage(t *testing.T) {
	audio := append([]byte("ID3"), bytes.Repeat([]byte{0}, 100)...)
	c := NewClient(&http.Client{Transport: creationTransport(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "/api/v3/tts/unidirectional/sse", r.URL.Path)
		require.Equal(t, "*", r.Header.Get("X-Control-Require-Usage-Tokens-Return"))
		var request struct {
			Params struct {
				Text string `json:"text"`
			} `json:"req_params"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, "你 好\n", request.Params.Text)
		body := "data: {\"code\":0,\"data\":\"" + base64.StdEncoding.EncodeToString(audio[:7]) + "\",\"usage\":{\"text_words\":1}}\n\n" +
			"data: {\"code\":0,\"data\":\"" + base64.StdEncoding.EncodeToString(audio[7:]) + "\"}\n\n" +
			"data: {\"code\":20000000,\"usage\":{\"text_words\":4}}\n\n"
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	in := application.GenerationRequest{Config: creationConfig("volcengine_speech"), ModelID: "seed-tts-2.0", Capability: "audio.generate", Mode: "text2audio", Prompt: "你 好\n", OutputCount: 1, RequireSpeechTerminalUsage: true}
	got, err := c.Submit(t.Context(), in, "fixture-key")
	require.NoError(t, err)
	require.Equal(t, application.ProviderOutcomeCompleted, got.Outcome)
	require.JSONEq(t, `{"text_words":4}`, string(got.Usage))
	require.Equal(t, audio, got.Audio[0].Bytes)
}
