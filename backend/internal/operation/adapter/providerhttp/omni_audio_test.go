package providerhttp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestOmniAudioWireAndOriginalDuration(t *testing.T) {
	audio := append([]byte("ID3"), bytes.Repeat([]byte{0}, 100)...)
	calls := 0
	c := NewClient(&http.Client{Transport: creationTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://openspeech.bytedance.com/api/v3/tts/create" || r.Header.Get("X-Api-Key") != "fixture-key" || r.Header.Get("Authorization") != "" || r.Header.Get("X-Api-Resource-Id") != "" {
			t.Fatal("wrong Seed-Audio route or authentication")
		}
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["model"] != "seed-audio-1.0" || body["text_prompt"] != "让 @音频1 说你好" {
			t.Fatal("wrong Seed-Audio envelope")
		}
		refs := body["references"].([]any)
		if len(refs) != 1 || refs[0].(map[string]any)["audio_url"] != "https://assets.example/voice.mp3" {
			t.Fatal("lost ordered reference")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"audio":"` + base64.StdEncoding.EncodeToString(audio) + `","duration":6.25,"original_duration":12.5,"subtitle":{"text":"你好","sentences":[{"start_time":0,"end_time":1000,"text":"你好","words":[{"start_time":0,"end_time":1000,"text":"你好"}]}]}}`))}, nil
	})})
	in := omniAudioInput()
	in.References = []application.ProviderReference{{Role: "reference_audio", MediaKind: "audio", URL: "https://assets.example/voice.mp3"}}
	got, err := c.Submit(t.Context(), in, "fixture-key")
	if err != nil || got.State != "succeeded" || len(got.Audio) != 1 || !bytes.Equal(got.Audio[0].Bytes, audio) || !strings.Contains(string(got.Usage), `"original_duration":12.5`) || calls != 1 {
		t.Fatalf("Seed-Audio response: %+v %v calls=%d", got, err, calls)
	}
}

func omniAudioInput() application.GenerationRequest {
	return application.GenerationRequest{Config: creationConfig("volcengine_speech"), ModelID: "seed-audio-1.0", Capability: "audio.generate", Mode: "omni_audio", Prompt: "让 @音频1 说你好", OutputCount: 1, Params: map[string]any{"enable_subtitle": true}}
}

func TestOmniAudioRejectsMixedReferencesAndNeverRetries(t *testing.T) {
	calls := 0
	c := NewClient(&http.Client{Transport: creationTransport(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("timeout") })})
	in := omniAudioInput()
	in.References = []application.ProviderReference{{Role: "reference_audio", MediaKind: "audio", URL: "https://assets.example/a.mp3"}, {Role: "reference_image", MediaKind: "image", URL: "https://assets.example/i.png"}}
	got, _ := c.Submit(t.Context(), in, "fixture-key")
	if got.Outcome != "not_submitted" || calls != 0 {
		t.Fatal("mixed references reached provider")
	}
	in.References = nil
	in.Prompt = "生成海浪声"
	got, _ = c.Submit(t.Context(), in, "fixture-key")
	if got.Outcome != "unknown" || calls != 1 {
		t.Fatal("ambiguous request was retried or misclassified")
	}
}
