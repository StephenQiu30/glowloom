package subject_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/adapter/doubao"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
)

type voiceTransport func(*http.Request) (*http.Response, error)

func (f voiceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func voiceProtocolInput() subject.VoiceSynthesisInput {
	return subject.VoiceSynthesisInput{Config: json.RawMessage(`{"adapter_key":"volcengine_speech","base_url":"https://openspeech.bytedance.com/api/v3","region":"domestic","capabilities":["audio"],"timeout_seconds":120,"concurrency_limit":2,"rate_limit_per_min":30}`), RequestID: uuid.New(), ResourceID: "seed-tts-2.0", Text: "你好，织光。", VoiceKey: "zh_female_vv_uranus_bigtts", SpeechRate: 0}
}

func TestDoubaoVoiceExactProtocolAndTerminalCompletion(t *testing.T) {
	input := voiceProtocolInput()
	const key = "synthetic-fixture-secret"
	var calls int
	client := doubao.NewClient(&http.Client{Transport: voiceTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" || r.URL.String() != "https://openspeech.bytedance.com/api/v3/tts/unidirectional/sse" || r.Header.Get("X-Api-Key") != key || r.Header.Get("X-Api-Resource-Id") != input.ResourceID || r.Header.Get("X-Api-Request-Id") != input.RequestID.String() || r.Header.Get("X-Api-App-Id") != "" || r.Header.Get("Authorization") != "" {
			t.Fatal("speech request did not use its independent API-Key protocol")
		}
		var body struct {
			ReqParams struct {
				Text, Speaker string
				AudioParams   struct {
					Format     string
					SampleRate int `json:"sample_rate"`
					SpeechRate int `json:"speech_rate"`
				} `json:"audio_params"`
			} `json:"req_params"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.ReqParams.Text != input.Text || body.ReqParams.Speaker != input.VoiceKey || body.ReqParams.AudioParams.Format != "mp3" || body.ReqParams.AudioParams.SampleRate != 24000 {
			t.Fatal("incorrect frozen synthesis parameters")
		}
		frames := `{"code":0,"data":"` + base64.StdEncoding.EncodeToString([]byte("ID3real-")) + `"}` + "\n" + `{"code":0,"data":null,"sentence":{"text":"你好"}}` + "\n" + `{"code":0,"data":"` + base64.StdEncoding.EncodeToString([]byte("audio")) + `"}` + "\n" + `{"code":20000000,"data":null,"usage":{"text_words":6}}`
		return &http.Response{StatusCode: 200, Header: http.Header{"X-Tt-Logid": []string{"synthetic-log"}, "Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(voiceSSE(frames)))}, nil
	})})
	output, err := client.Synthesize(t.Context(), input, key)
	if err != nil || string(output.Audio) != "ID3real-audio" || output.TerminalCode != 20000000 || output.MIMEType != "audio/mpeg" || output.ProviderLogID != "synthetic-log" || calls != 1 {
		t.Fatal("actual concatenated terminal audio", err)
	}
}

func TestDoubaoVoiceNeverAcceptsPartialOrAmbiguousExecution(t *testing.T) {
	for _, test := range []struct{ name, frames string }{
		{"missing terminal", `{"code":0,"data":"SUQzYXVkaW8="}`},
		{"truncated frame", `{"code":0,"data":"SUQzYXVkaW8="`},
		{"invalid base64", `{"code":0,"data":"%%%"}{"code":20000000}`},
		{"no audio", `{"code":20000000,"data":null}`},
		{"missing code", `{"data":"SUQzYXVkaW8="}{"code":20000000}`},
		{"audio after terminal", `{"code":0,"data":"SUQzYXVkaW8="}{"code":20000000}{"code":0,"data":"YQ=="}`},
		{"partial then error", `{"code":0,"data":"SUQzYXVkaW8="}{"code":55000000,"message":"private upstream details"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			client := doubao.NewClient(&http.Client{Transport: voiceTransport(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(voiceSSE(test.frames)))}, nil
			})})
			output, err := client.Synthesize(t.Context(), voiceProtocolInput(), "synthetic-fixture-secret")
			if !errors.Is(err, subject.ErrVoiceExecutionUnknown) || len(output.Audio) != 0 || calls != 1 || strings.Contains(err.Error(), "private upstream details") {
				t.Fatal("ambiguous paid execution was accepted or retried", err)
			}
		})
	}
	client := doubao.NewClient(&http.Client{Transport: voiceTransport(func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })})
	if _, err := client.Synthesize(t.Context(), voiceProtocolInput(), "synthetic-fixture-secret"); !errors.Is(err, subject.ErrVoiceExecutionUnknown) {
		t.Fatal("transport failure must be unknown", err)
	}
}

func TestDoubaoVoiceRejectsRedirectWithoutLeakingKey(t *testing.T) {
	calls := 0
	client := doubao.NewClient(&http.Client{Transport: voiceTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://untrusted.invalid/collect"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	if _, err := client.Synthesize(t.Context(), voiceProtocolInput(), "synthetic-fixture-secret"); !errors.Is(err, subject.ErrVoiceExecutionUnknown) || calls != 1 {
		t.Fatal("redirect followed or not rejected", err)
	}
}

func voiceSSE(frames string) string {
	d := json.NewDecoder(strings.NewReader(frames))
	var out strings.Builder
	for {
		var frame json.RawMessage
		if err := d.Decode(&frame); err != nil {
			if err != io.EOF {
				return out.String() + "data: " + frames + "\n\n"
			}
			break
		}
		out.WriteString("data: ")
		out.Write(frame)
		out.WriteString("\n\n")
	}
	return out.String()
}

func TestDoubaoVoiceStrictSSERejectsTruncatedEventAndWrongProtocol(t *testing.T) {
	for _, fixture := range []struct{ kind, body string }{{"text/event-stream", "data: {\"code\":0,\"data\":\"SUQzYXVkaW8=\"}\n\ndata: {\"code\":20000000}"}, {"application/json", `{"code":0,"data":"SUQzYXVkaW8="}{"code":20000000}`}, {"text/event-stream", "data: {\"code\":0,\"data\":\"SUQzYXVkaW8=\"}\n\ndata: [DONE]\n\n"}} {
		calls := 0
		client := doubao.NewClient(&http.Client{Transport: voiceTransport(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{fixture.kind}}, Body: io.NopCloser(strings.NewReader(fixture.body))}, nil
		})})
		got, err := client.Synthesize(t.Context(), voiceProtocolInput(), "synthetic-key")
		if !errors.Is(err, subject.ErrVoiceExecutionUnknown) || len(got.Audio) > 0 || calls != 1 {
			t.Fatal("partial SSE event or protocol mismatch completed a paid job", err)
		}
	}
}
