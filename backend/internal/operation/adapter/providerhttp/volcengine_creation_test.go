package providerhttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestMusicSigningFixedVector(t *testing.T) {
	// Independently calculated with Python hashlib/hmac from the official
	// canonical-request contract; these are synthetic credentials.
	body := []byte(`{"TaskID":"fixture-task"}`)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://open.volcengineapi.com/?Version=2024-08-12&Action=QuerySong", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	signMusicRequest(req, body, "fixture-ak", "fixture-sk", time.Date(2026, 10, 29, 1, 2, 3, 0, time.UTC))
	want := "HMAC-SHA256 Credential=fixture-ak/20261029/cn-beijing/imagination/request, SignedHeaders=content-type;host;x-content-sha256;x-date, Signature=ed8c32047ddc72c28ff6697a0451da893a1245ff4b4cbf8feea705238ce4bf29"
	if req.Header.Get("Authorization") != want {
		t.Fatal("signature differs from fixed vector")
	}
}

func TestMusicInvalidInputDoesNotReachProvider(t *testing.T) {
	c := NewClient(&http.Client{Transport: creationTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid input reached provider")
		return nil, errors.New("unexpected transport")
	})})
	for _, tc := range []struct{ key, prompt string }{
		{"Bearer fixture", "清晨的钢琴歌曲"},
		{`{"access_key_id":"fixture-ak","secret_access_key":"fixture-sk"}`, strings.Repeat("春", 701)},
	} {
		got, _ := c.Submit(context.Background(), application.GenerationRequest{Config: creationConfig("volcengine_music"), ModelID: "v5.0", Capability: "audio.generate", Mode: "music_song", Prompt: tc.prompt, Params: map[string]any{"duration_ms": 30000}, OutputCount: 1}, tc.key)
		if got.Outcome != "not_submitted" {
			t.Fatalf("invalid input marked ambiguous: %+v", got)
		}
	}
}

type creationTransport func(*http.Request) (*http.Response, error)

func (f creationTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func creationConfig(adapter string) catalog.ProviderConfig {
	for _, p := range catalog.BuiltinPresets() {
		if p.Config.AdapterKey == adapter {
			return p.Config
		}
	}
	return catalog.ProviderConfig{}
}

func TestCreationQueryDoesNotTreatErrorPayloadAsSuccess(t *testing.T) {
	for _, tc := range []struct{ adapter, body, want string }{
		{"volcengine_music", `{"Code":0,"ResponseMetadata":{"Error":null},"Result":{"Status":2,"TaskID":"task-123","SongDetail":{"AudioUrl":"https://v1-default.douyinvod.com/music.mp3","Duration":30.125}}}`, "succeeded"},
		{"volcengine_music", `{"Code":0,"Result":{"Status":3,"SongDetail":{"AudioUrl":"https://v1-default.douyinvod.com/stale.mp3","Duration":30}}}`, "failed"},
		{"volcengine_music", `{"Code":0,"Result":{"Status":2,"TaskID":"other-task","SongDetail":{"AudioUrl":"https://v1-default.douyinvod.com/music.mp3","Duration":30}}}`, "unknown"},
		{"volcengine_music", `{"Code":0,"Result":{"SongDetail":{"AudioUrl":"https://v1-default.douyinvod.com/music.mp3","Duration":30}}}`, "unknown"},
		{"volcengine_avatar", `{"code":10000,"ResponseMetadata":{"Error":null},"data":{"status":"done","video_url":"https://cdn.vendor.example/result.mp4"}}`, "succeeded"},
		{"volcengine_avatar", `{"code":10000,"data":{"status":"done"}}`, "unknown"},
		{"volcengine_avatar", `{"code":50215,"data":null}`, "failed"},
		{"volcengine_motion", `{"code":10000,"data":{"status":"not_found"}}`, "not_found"},
		{"volcengine_motion", `{"code":10000,"data":{"status":"expired"}}`, "unknown"},
		{"volcengine_motion", `{"code":10000,"data":{"status":"done","video_url":"https://127.0.0.1/internal"}}`, "unknown"},
	} {
		t.Run(tc.adapter+tc.want, func(t *testing.T) {
			c := NewClient(&http.Client{Transport: creationTransport(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})})
			key := "fixture-key"
			if tc.adapter == "volcengine_music" {
				key = `{"access_key_id":"fixture-ak","secret_access_key":"fixture-sk"}`
			}
			got, err := c.Query(context.Background(), creationConfig(tc.adapter), "task-123", key)
			if err != nil || got.State != tc.want {
				t.Fatalf("query = %+v %v; want %s", got, err, tc.want)
			}
			if tc.want == "succeeded" && got.TaskID != "task-123" {
				t.Fatal("lost original task identity")
			}
			if tc.want != "succeeded" && len(got.URLs) > 0 {
				t.Fatal("non-success exposed output")
			}
		})
	}
}

func TestCreationSubmitTransportFailureIsNeverRetried(t *testing.T) {
	calls := 0
	c := NewClient(&http.Client{Transport: creationTransport(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("fixture timeout") })})
	got, _ := c.Submit(context.Background(), application.GenerationRequest{Config: creationConfig("volcengine_music"), ModelID: "v5.0", Capability: "audio.generate", Mode: "music_song", Prompt: "清晨的钢琴歌曲", Params: map[string]any{"duration_ms": 30000}, OutputCount: 1}, `{"access_key_id":"fixture-ak","secret_access_key":"fixture-sk"}`)
	if calls != 1 || got.Outcome != "unknown" {
		t.Fatalf("ambiguous submit = %+v, calls=%d", got, calls)
	}
}

func TestVisualCreationWireAndQuery(t *testing.T) {
	for _, tc := range []struct{ adapter, model, mode, path, role, kind string }{
		{"volcengine_avatar", "jimeng_realman_avatar_picture_omni_v15", "avatar", "submit_task", "driving_audio", "audio"},
		{"volcengine_motion", "jimeng_dreamactor_m20_gen_video", "motion", "submit_task_v2", "driving_video", "video"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			calls := 0
			c := NewClient(&http.Client{Transport: creationTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Host != "openapi.cv.volces.com" || r.URL.Path != "/api/common/v3/"+tc.path || r.Header.Get("Authorization") != "Bearer fixture-key" {
					t.Errorf("incorrect route/auth: %s", r.URL)
				}
				body, _ := io.ReadAll(r.Body)
				if !strings.Contains(string(body), tc.model) || strings.Contains(string(body), "duration") {
					t.Errorf("incorrect payload: %s", body)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":10000,"data":{"task_id":"task-123"}}`))}, nil
			})})
			input := application.GenerationRequest{Config: creationConfig(tc.adapter), ModelID: tc.model, Capability: "video.generate", Mode: tc.mode, OutputCount: 1, Params: map[string]any{}, References: []application.ProviderReference{{Role: "character_image", MediaKind: "image", URL: "https://assets.example/role.png"}, {Role: tc.role, MediaKind: tc.kind, URL: "https://assets.example/drive"}}}
			got, err := c.Submit(context.Background(), input, "fixture-key")
			if err != nil || got.Outcome != "accepted" || got.TaskID != "task-123" || calls != 1 {
				t.Fatalf("submit: %+v %v calls=%d", got, err, calls)
			}
		})
	}
}

func TestMusicUsesSignedVersionedProtocol(t *testing.T) {
	for _, tc := range []struct{ mode, action, field string }{
		{"music_song", "GenSongForTime", "ModelVersion"}, {"music_instrumental", "GenBGMForTime", "Version"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			c := NewClient(&http.Client{Transport: creationTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "open.volcengineapi.com" || r.URL.Query().Get("Action") != tc.action || r.URL.Query().Get("Version") != "2024-08-12" || !strings.Contains(r.Header.Get("Authorization"), "/cn-beijing/imagination/request") {
					t.Errorf("incorrect signed request: %s", r.URL)
				}
				body, _ := io.ReadAll(r.Body)
				if !strings.Contains(string(body), `"`+tc.field+`":"v5.0"`) {
					t.Errorf("incorrect model field: %s", body)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"Code":0,"Result":{"TaskID":"music-123"}}`))}, nil
			})})
			got, err := c.Submit(context.Background(), application.GenerationRequest{Config: creationConfig("volcengine_music"), ModelID: "v5.0", Capability: "audio.generate", Mode: tc.mode, Prompt: "清晨咖啡馆里的温柔钢琴音乐", Params: map[string]any{"duration_ms": 60000}, OutputCount: 1}, `{"access_key_id":"fixture-ak","secret_access_key":"fixture-sk"}`)
			if err != nil || got.Outcome != "accepted" {
				t.Fatalf("submit: %+v %v", got, err)
			}
		})
	}
}
