package modelgateway_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/modelgateway"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (r roundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return r(req) }
func TestChatToolResultAndNewCallRemainStructured(t *testing.T) {
	input := json.RawMessage(`{"model_key":"fixture","max_tokens":200,"messages":[{"role":"system","content":"limit scope"},{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"context","arguments":"{}"}}]},{"role":"tool","tool_call_id":"a","content":"saved fact"},{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}]}],"tools":[{"name":"context","description":"read context","parameters":{"type":"object"}}]}`)
	calls := 0
	transport := roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "Bearer fixture", r.Header.Get("Authorization"))
		require.Equal(t, "/api/v1/chat/completions", r.URL.Path)
		raw, _ := io.ReadAll(r.Body)
		var req map[string]any
		require.NoError(t, json.Unmarshal(raw, &req))
		require.Equal(t, "upstream", req["model"])
		require.Len(t, req["messages"], 4)
		require.Len(t, req["tools"], 1)
		require.Contains(t, string(raw), "tool_call_id")
		require.Contains(t, string(raw), "data:image/png")
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"b","type":"function","function":{"name":"context","arguments":"{\"x\":1}"}}]}}],"usage":{"prompt_tokens":20,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":3}}}`))}, nil
	})
	c := catalog.ProviderConfig{AdapterKey: "openrouter", BaseURL: "https://openrouter.ai/api/v1", Region: "overseas", Capabilities: []string{"text"}, TimeoutSeconds: 30, ConcurrencyLimit: 1, RateLimitPerMin: 10}
	out, err := modelgateway.NewChatClient(&http.Client{Transport: transport}).Complete(context.Background(), c, "fixture", "upstream", input)
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, "tool_use", out.StopReason)
	require.Equal(t, "context", out.ToolCalls[0].Name)
	require.EqualValues(t, 3, *out.Usage.CacheReadTokens)
}

func TestChatRejectsMalformedTranscriptBeforeProviderRequest(t *testing.T) {
	config := catalog.ProviderConfig{AdapterKey: "openrouter", BaseURL: "https://openrouter.ai/api/v1", Region: "overseas", Capabilities: []string{"text"}, TimeoutSeconds: 30, ConcurrencyLimit: 1, RateLimitPerMin: 10}
	client := modelgateway.NewChatClient(&http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid dialogue reached provider")
		return nil, nil
	})})
	for _, raw := range []string{
		`{"model_key":"fixture","max_tokens":10,"messages":[{"role":"user","content":99}]}`,
		`{"model_key":"fixture","max_tokens":10,"messages":[{"role":"tool","content":"unbound"}]}`,
		`{"model_key":"fixture","max_tokens":10,"messages":[{"role":"user","content":null}]}`,
		`{"model_key":"fixture","max_tokens":10,"messages":[{"role":"user","content":[{"type":"image_url"}]}]}`,
		`{"model_key":"fixture","max_tokens":10,"messages":[{"role":"assistant","tool_calls":[{"id":"x","type":"function","function":{"name":"read","arguments":"[]"}}]}]}`,
		`{"model_key":"fixture","max_tokens":10,"messages":[{"role":"user","content":"valid"}],"provider_key":"injected"}`,
	} {
		_, err := client.Complete(t.Context(), config, "fixture", "upstream", json.RawMessage(raw))
		require.Error(t, err)
	}
}

func TestChatTransportFailureDoesNotRetryPaidRequest(t *testing.T) {
	calls := 0
	config := catalog.ProviderConfig{AdapterKey: "openrouter", BaseURL: "https://openrouter.ai/api/v1", Region: "overseas", Capabilities: []string{"text"}, TimeoutSeconds: 30, ConcurrencyLimit: 1, RateLimitPerMin: 10}
	client := modelgateway.NewChatClient(&http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF })})
	_, err := client.Complete(t.Context(), config, "fixture", "upstream", json.RawMessage(`{"model_key":"fixture","max_tokens":10,"messages":[{"role":"user","content":"fixture"}]}`))
	require.Error(t, err)
	require.Equal(t, 1, calls)
}

func TestChatStalledResponseExpiresWithoutPaidRetry(t *testing.T) {
	for _, stage := range []string{"headers", "body"} {
		t.Run(stage, func(t *testing.T) {
			var hits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				_ = r.Body.Close()
				if stage == "body" {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"choices":`))
					w.(http.Flusher).Flush()
				}
				select {
				case <-r.Context().Done():
				case <-time.After(2 * time.Second):
				}
			}))
			defer server.Close()
			target, err := url.Parse(server.URL)
			require.NoError(t, err)
			transport := http.DefaultTransport.(*http.Transport).Clone()
			defer transport.CloseIdleConnections()
			proxy := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				request := r.Clone(r.Context())
				request.URL.Scheme = target.Scheme
				request.URL.Host = target.Host
				return transport.RoundTrip(request)
			})}
			client := modelgateway.NewChatClient(proxy, 100*time.Millisecond)
			config := catalog.ProviderConfig{AdapterKey: "openrouter", BaseURL: "https://openrouter.ai/api/v1", Region: "overseas", Capabilities: []string{"text"}, TimeoutSeconds: 30, ConcurrencyLimit: 1, RateLimitPerMin: 10}
			ctx, cancel := context.WithTimeout(t.Context(), 1500*time.Millisecond)
			defer cancel()
			start := time.Now()
			_, err = client.Complete(ctx, config, "fixture", "upstream", json.RawMessage(`{"model_key":"fixture","max_tokens":10,"messages":[{"role":"user","content":"fixture"}]}`))
			require.Error(t, err)
			require.Less(t, time.Since(start), 1000*time.Millisecond)
			require.EqualValues(t, 1, hits.Load())
		})
	}
}

func TestChatBodyProgressRenewsIdleWindow(t *testing.T) {
	const response = `{"choices":[{"finish_reason":"stop","message":{"content":"fixture reply"}}],"usage":{"prompt_tokens":7,"completion_tokens":3}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		for start := 0; start < len(response); start += 30 {
			end := min(start+30, len(response))
			_, _ = w.Write([]byte(response[start:end]))
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(40 * time.Millisecond):
			}
		}
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	require.NoError(t, err)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	proxy := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		request := r.Clone(r.Context())
		request.URL.Scheme = target.Scheme
		request.URL.Host = target.Host
		return transport.RoundTrip(request)
	})}
	config := catalog.ProviderConfig{AdapterKey: "openrouter", BaseURL: "https://openrouter.ai/api/v1", Region: "overseas", Capabilities: []string{"text"}, TimeoutSeconds: 30, ConcurrencyLimit: 1, RateLimitPerMin: 10}
	out, err := modelgateway.NewChatClient(proxy, 100*time.Millisecond).Complete(t.Context(), config, "fixture", "upstream", json.RawMessage(`{"model_key":"fixture","max_tokens":10,"messages":[{"role":"user","content":"fixture"}]}`))
	require.NoError(t, err)
	require.Equal(t, "fixture reply", out.Output)
}
