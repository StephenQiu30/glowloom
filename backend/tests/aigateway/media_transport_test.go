package aigateway_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	catalog "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/provider"
	app "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
)

const mediaPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

func mediaDispatch(t *testing.T, adapter, capability, mode, model string) app.Dispatch {
	t.Helper()
	spec, ok := catalog.GetAdapterSpec(adapter)
	require.True(t, ok)
	config := catalog.ProviderConfig{AdapterKey: adapter, BaseURL: spec.AllowedBaseURL, Region: spec.AllowedRegion, Capabilities: []string{strings.TrimSuffix(capability, ".generate")}, TimeoutSeconds: 5, ConcurrencyLimit: 1, RateLimitPerMin: 60}
	require.NoError(t, config.Validate())
	configJSON, err := json.Marshal(config)
	require.NoError(t, err)
	price := domain.Price{Request: 1234, Currency: "CNY", FX: "1", Multiplier: "1", Discount: "1"}
	m := domain.Model{ID: uuid.New(), Name: "media-product", Protocol: "media", Capability: capability, Modes: []string{mode}, Price: price, MaxInput: 100, MaxOutput: 100}
	in := domain.MediaInput{Model: m.Name, Capability: capability, Mode: mode, Prompt: "synthetic input", Params: map[string]any{}, OutputCount: 1}
	raw, err := json.Marshal(in)
	require.NoError(t, err)
	return app.Dispatch{Call: domain.Call{ID: uuid.New(), Scope: uuid.New(), Actor: uuid.New(), Project: uuid.New(), Model: m, Raw: raw}, Route: domain.Route{ModelID: m.ID, Protocol: "media", BaseURL: config.BaseURL, UpstreamModel: model, Purchase: price, DriverConfig: configJSON}, Send: true}
}

func mediaClient(rt transportFunc) *provider.Client {
	return provider.New(keyFixture{}, &http.Client{Transport: rt}).WithLookup(func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	})
}

func mediaResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func mediaChange(t *testing.T, d *app.Dispatch, change func(*domain.MediaInput)) {
	t.Helper()
	var in domain.MediaInput
	require.NoError(t, json.Unmarshal(d.Call.Raw, &in))
	change(&in)
	var err error
	d.Call.Raw, err = json.Marshal(in)
	require.NoError(t, err)
}

func TestMediaImageCompletedUsesSelectedRouteAndFixedPrice(t *testing.T) {
	d := mediaDispatch(t, "openai", "image.generate", "text2image", "gpt-image-1")
	count := 0
	c := mediaClient(func(r *http.Request) (*http.Response, error) {
		count++
		require.Equal(t, "8.8.8.8:443", r.URL.Host)
		require.Equal(t, "api.openai.com", r.Host)
		require.Equal(t, "/v1/images/generations", r.URL.Path)
		require.Equal(t, "Bearer fixture-only", r.Header.Get("Authorization"))
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Equal(t, "gpt-image-1", payload["model"])
		return mediaResponse(200, fmt.Sprintf(`{"data":[{"b64_json":%q}],"usage":{"input_tokens":9,"output_tokens":2}}`, mediaPNG)), nil
	})
	receipt, err := c.Send(context.Background(), d, nil)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.False(t, receipt.Pending)
	require.Equal(t, domain.Usage{}, receipt.Usage)
	require.Equal(t, int64(1234), receipt.PurchaseMicros)
	var result operation.GenerationResult
	require.NoError(t, json.Unmarshal(receipt.Body, &result))
	require.Len(t, result.Images, 1)
	require.JSONEq(t, `{"input_tokens":9,"output_tokens":2}`, string(result.Usage))
	require.NotContains(t, string(receipt.Body), "fixture-only")
}

func TestMediaArkAcceptedAndQueryNeverResubmits(t *testing.T) {
	d := mediaDispatch(t, "volcengine_ark", "video.generate", "text2video", "seedance-fixture")
	methods := []string{}
	c := mediaClient(func(r *http.Request) (*http.Response, error) {
		methods = append(methods, r.Method)
		if r.Method == http.MethodPost {
			require.Equal(t, "/api/v3/contents/generations/tasks", r.URL.Path)
			return mediaResponse(200, `{"id":"task_fixture"}`), nil
		}
		require.Equal(t, "/api/v3/contents/generations/tasks/task_fixture", r.URL.Path)
		return mediaResponse(200, `{"id":"task_fixture","status":"succeeded","content":{"video_url":"https://fixture.tos-cn-beijing.volces.com/out.mp4"},"usage":{"total_tokens":50}}`), nil
	})
	receipt, err := c.Send(context.Background(), d, nil)
	require.NoError(t, err)
	require.True(t, receipt.Pending)
	require.Equal(t, "task_fixture", receipt.UpstreamID)
	d.PollTask = receipt.UpstreamID
	// Polling only needs the original immutable route and task; it must not resolve inputs again.
	d.Call.Raw = nil
	receipt, err = c.Send(context.Background(), d, nil)
	require.NoError(t, err)
	require.False(t, receipt.Pending)
	require.Equal(t, int64(1234), receipt.PurchaseMicros)
	require.Equal(t, []string{"POST", "GET"}, methods)
	var result operation.GenerationResult
	require.NoError(t, json.Unmarshal(receipt.Body, &result))
	require.Len(t, result.URLs, 1)
	require.JSONEq(t, `{"total_tokens":50}`, string(result.Usage))
}

func TestMediaTransportAmbiguityNeverRetries(t *testing.T) {
	for _, status := range []int{302, 429, 500, 200} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			count := 0
			c := mediaClient(func(*http.Request) (*http.Response, error) {
				count++
				r := mediaResponse(status, `{"error":{"message":"fixture-only"}}`)
				r.Header.Set("Location", "https://other.example/")
				return r, nil
			})
			_, err := c.Send(context.Background(), mediaDispatch(t, "openai", "image.generate", "text2image", "gpt-image-1"), nil)
			require.ErrorIs(t, err, domain.ErrUnknown)
			require.Equal(t, 1, count)
			require.NotContains(t, err.Error(), "fixture-only")
		})
	}
	count := 0
	c := mediaClient(func(*http.Request) (*http.Response, error) { count++; return nil, context.DeadlineExceeded })
	_, err := c.Send(context.Background(), mediaDispatch(t, "openai", "image.generate", "text2image", "gpt-image-1"), nil)
	require.ErrorIs(t, err, domain.ErrUnknown)
	require.Equal(t, 1, count)
}

func TestMediaPreflightAndPrivateAddressDoNotSend(t *testing.T) {
	for _, scenario := range []string{"base", "config", "capability", "parameter", "count", "private", "missing_resolver", "resolver_failure", "resolver_dropped_reference", "resolver_changed_role"} {
		t.Run(scenario, func(t *testing.T) {
			d := mediaDispatch(t, "volcengine_ark", "image.generate", "text2image", "seedream-fixture")
			count := 0
			c := mediaClient(func(*http.Request) (*http.Response, error) { count++; return mediaResponse(500, "{}"), nil })
			switch scenario {
			case "base":
				d.Route.BaseURL = "https://different.example/api/v3"
			case "config":
				d.Route.DriverConfig = json.RawMessage(`{}`)
			case "capability":
				mediaChange(t, &d, func(in *domain.MediaInput) { in.Capability = "video.generate" })
			case "parameter":
				mediaChange(t, &d, func(in *domain.MediaInput) { in.Params["unreviewed"] = true })
			case "count":
				mediaChange(t, &d, func(in *domain.MediaInput) { in.OutputCount = 2 })
			case "private":
				c = c.WithLookup(func(context.Context, string, string) ([]netip.Addr, error) {
					return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
				})
			default:
				d.Call.Model.Modes = []string{"image2image"}
				mediaChange(t, &d, func(in *domain.MediaInput) {
					in.Mode = "image2image"
					in.References = []domain.Reference{{ID: uuid.New(), Role: "reference"}}
				})
				if scenario != "missing_resolver" {
					c = c.WithMediaReferences(func(context.Context, app.Dispatch, []domain.Reference) ([]operation.ProviderReference, error) {
						if scenario == "resolver_failure" {
							return nil, errors.New("private owned object")
						}
						if scenario == "resolver_changed_role" {
							return []operation.ProviderReference{{Role: "subject", MediaKind: "image", URL: "https://owned.example/original"}}, nil
						}
						return nil, nil
					})
				}
			}
			_, err := c.Send(context.Background(), d, nil)
			require.ErrorIs(t, err, app.ErrNotSent)
			require.Zero(t, count)
		})
	}
}

func TestMediaOwnedReferencesReachArkWithoutDropping(t *testing.T) {
	d := mediaDispatch(t, "volcengine_ark", "image.generate", "image2image", "seedream-fixture")
	ref := domain.Reference{ID: uuid.New(), BindingID: uuid.New(), Version: "original-v1", Role: "reference"}
	mediaChange(t, &d, func(in *domain.MediaInput) { in.References = []domain.Reference{ref} })
	resolved := false
	c := mediaClient(func(r *http.Request) (*http.Response, error) {
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Equal(t, []any{"https://owned.example/original-v1"}, payload["image"])
		return mediaResponse(200, fmt.Sprintf(`{"data":[{"b64_json":%q}]}`, mediaPNG)), nil
	}).WithMediaReferences(func(_ context.Context, dispatched app.Dispatch, refs []domain.Reference) ([]operation.ProviderReference, error) {
		resolved = true
		require.Equal(t, d.Call.Project, dispatched.Call.Project)
		require.Equal(t, []domain.Reference{ref}, refs)
		return []operation.ProviderReference{{Role: "reference", MediaKind: "image", URL: "https://owned.example/original-v1"}}, nil
	})
	_, err := c.Send(context.Background(), d, nil)
	require.NoError(t, err)
	require.True(t, resolved)
}

func TestMediaCustomGatewayKeepsOriginalHostWithPublicPin(t *testing.T) {
	d := mediaDispatch(t, "openai", "image.generate", "text2image", "gpt-image-1")
	var config catalog.ProviderConfig
	require.NoError(t, json.Unmarshal(d.Route.DriverConfig, &config))
	config.BaseURL = "https://8.8.8.8/v1"
	d.Route.BaseURL = config.BaseURL
	d.Route.DriverConfig, _ = json.Marshal(config)
	c := mediaClient(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "8.8.8.8", r.Host)
		require.Equal(t, "8.8.8.8:443", r.URL.Host)
		return mediaResponse(200, fmt.Sprintf(`{"data":[{"b64_json":%q}]}`, mediaPNG)), nil
	})
	_, err := c.Send(context.Background(), d, nil)
	require.NoError(t, err)
}

func TestMediaSpeechPreservesDeclaredHeadersAndTerminalUsage(t *testing.T) {
	d := mediaDispatch(t, "volcengine_speech", "audio.generate", "text2audio", "seed-tts-2.0")
	c := mediaClient(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "/api/v3/tts/unidirectional/sse", r.URL.Path)
		require.Equal(t, "fixture-only", r.Header.Get("X-Api-Key"))
		require.Empty(t, r.Header.Get("Authorization"))
		require.Equal(t, "seed-tts-2.0", r.Header.Get("X-Api-Resource-Id"))
		require.Equal(t, d.Call.ID.String(), r.Header.Get("X-Api-Request-Id"))
		require.Equal(t, "*", r.Header.Get("X-Control-Require-Usage-Tokens-Return"))
		mp3 := base64.StdEncoding.EncodeToString([]byte("ID3" + strings.Repeat("\x00", 64)))
		res := mediaResponse(200, fmt.Sprintf("data: {\"code\":0,\"data\":%q}\n\ndata: {\"code\":20000000,\"usage\":{\"text_words\":15}}\n\n", mp3))
		res.Header.Set("Content-Type", "text/event-stream")
		return res, nil
	})
	receipt, err := c.Send(context.Background(), d, nil)
	require.NoError(t, err)
	var result operation.GenerationResult
	require.NoError(t, json.Unmarshal(receipt.Body, &result))
	require.Len(t, result.Audio, 1)
	require.JSONEq(t, `{"text_words":15}`, string(result.Usage))
}

func TestMediaDialEvidenceIsNotSent(t *testing.T) {
	c := mediaClient(func(*http.Request) (*http.Response, error) {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("synthetic refused")}
	})
	_, err := c.Send(context.Background(), mediaDispatch(t, "openai", "image.generate", "text2image", "gpt-image-1"), nil)
	require.ErrorIs(t, err, app.ErrNotSent)
	c = mediaClient(func(r *http.Request) (*http.Response, error) {
		httptrace.ContextClientTrace(r.Context()).GotConn(httptrace.GotConnInfo{})
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("ambiguous after connect")}
	})
	_, err = c.Send(context.Background(), mediaDispatch(t, "openai", "image.generate", "text2image", "gpt-image-1"), nil)
	require.ErrorIs(t, err, domain.ErrUnknown)
}

func TestMediaFailedOrUnknownPollRequiresReconciliation(t *testing.T) {
	for _, state := range []string{"failed", "cancelled", "expired", "unknown"} {
		t.Run(state, func(t *testing.T) {
			d := mediaDispatch(t, "volcengine_ark", "video.generate", "text2video", "seedance-fixture")
			d.PollTask = "task_fixture"
			c := mediaClient(func(r *http.Request) (*http.Response, error) {
				require.Equal(t, http.MethodGet, r.Method)
				return mediaResponse(200, fmt.Sprintf(`{"id":"task_fixture","status":%q}`, state)), nil
			})
			_, err := c.Send(context.Background(), d, nil)
			require.ErrorIs(t, err, domain.ErrUnknown)
		})
	}
}

func TestMediaPollPreflightFailureCannotAuthorizeAnotherSubmission(t *testing.T) {
	d := mediaDispatch(t, "volcengine_ark", "video.generate", "text2video", "seedance-fixture")
	d.PollTask = "task_fixture"
	d.Route.DriverConfig = json.RawMessage(`{}`)
	count := 0
	c := mediaClient(func(*http.Request) (*http.Response, error) { count++; return mediaResponse(500, "{}"), nil })
	_, err := c.Send(context.Background(), d, nil)
	require.ErrorIs(t, err, domain.ErrUnknown)
	require.Zero(t, count)
}

type mediaKeyFixture struct{ value string }

func (f mediaKeyFixture) Open(string, uuid.UUID, uuid.UUID, []byte) ([]byte, error) {
	return []byte(f.value), nil
}

func TestMediaMusicUsesOnlyInstalledSignatureHeaders(t *testing.T) {
	d := mediaDispatch(t, "volcengine_music", "audio.generate", "music_song", "v5.0")
	mediaChange(t, &d, func(in *domain.MediaInput) { in.Params["duration_ms"] = 30000 })
	count := 0
	c := provider.New(mediaKeyFixture{`{"api_key":"{\"access_key_id\":\"fixture-ak\",\"secret_access_key\":\"fixture-sk\"}"}`}, &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		count++
		require.Equal(t, "GenSongForTime", r.URL.Query().Get("Action"))
		require.Contains(t, r.Header.Get("Authorization"), "HMAC-SHA256 Credential=fixture-ak/")
		require.NotContains(t, r.Header.Get("Authorization"), "fixture-sk")
		require.NotEmpty(t, r.Header.Get("X-Date"))
		require.Len(t, r.Header.Get("X-Content-Sha256"), 64)
		return mediaResponse(200, `{"Code":0,"Result":{"TaskID":"music_fixture"}}`), nil
	})}).WithLookup(func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	})
	receipt, err := c.Send(context.Background(), d, nil)
	require.NoError(t, err)
	require.True(t, receipt.Pending)
	require.Equal(t, 1, count)
	require.NotContains(t, string(receipt.Body), "fixture-ak")
	require.NotContains(t, string(receipt.Body), "fixture-sk")
}

func TestMediaBoundedBodyRemainsUnknownAfterSend(t *testing.T) {
	d := mediaDispatch(t, "openai", "image.generate", "text2image", "gpt-image-1")
	count := 0
	c := mediaClient(func(*http.Request) (*http.Response, error) {
		count++
		return mediaResponse(200, strings.Repeat(" ", (16<<20)+1)), nil
	})
	_, err := c.Send(context.Background(), d, nil)
	require.ErrorIs(t, err, domain.ErrUnknown)
	require.Equal(t, 1, count)
}
