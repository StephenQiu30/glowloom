package modelgateway_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/modelgateway"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

func TestChatMapsOnlyExplicitDisabledThinkingForArk(t *testing.T) {
	for _, tc := range []struct {
		name, adapter, base, region, thinking string
		allowed                               bool
	}{
		{"disabled", "volcengine_ark", "https://ark.cn-beijing.volces.com/api/v3", "domestic", `"disabled"`, true},
		{"enabled", "volcengine_ark", "https://ark.cn-beijing.volces.com/api/v3", "domestic", `"enabled"`, false},
		{"auto", "volcengine_ark", "https://ark.cn-beijing.volces.com/api/v3", "domestic", `"auto"`, false},
		{"object", "volcengine_ark", "https://ark.cn-beijing.volces.com/api/v3", "domestic", `{"type":"disabled"}`, false},
		{"null", "volcengine_ark", "https://ark.cn-beijing.volces.com/api/v3", "domestic", `null`, false},
		{"other_adapter", "openrouter", "https://openrouter.ai/api/v1", "overseas", `"disabled"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := modelgateway.NewChatClient(&http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				var body map[string]json.RawMessage
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.JSONEq(t, `{"type":"disabled"}`, string(body["thinking"]))
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))}, nil
			})})
			cfg := catalog.ProviderConfig{AdapterKey: tc.adapter, BaseURL: tc.base, Region: catalog.Region(tc.region), Capabilities: []string{"text"}, TimeoutSeconds: 30, ConcurrencyLimit: 1, RateLimitPerMin: 10}
			raw := json.RawMessage(`{"model_key":"fixture","max_tokens":10,"params":{"thinking":` + tc.thinking + `},"messages":[{"role":"user","content":"hello"}]}`)
			_, err := client.Complete(t.Context(), cfg, "fixture-key", "arbitrary-frozen-upstream-model", raw)
			if tc.allowed {
				require.NoError(t, err)
				require.Equal(t, 1, calls)
			} else {
				require.Error(t, err)
				require.Zero(t, calls)
			}
		})
	}
}

func TestChatUsesOnlyReleaseIDContractField(t *testing.T) {
	calls := 0
	cfg := catalog.ProviderConfig{AdapterKey: "openrouter", BaseURL: "https://openrouter.ai/api/v1", Region: "overseas", Capabilities: []string{"text"}, TimeoutSeconds: 30, ConcurrencyLimit: 1, RateLimitPerMin: 10}
	client := modelgateway.NewChatClient(&http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))}, nil
	})})
	for _, field := range []string{"release_id", "model_release_id"} {
		_, err := client.Complete(t.Context(), cfg, "fixture", "upstream", json.RawMessage(`{"model_key":"fixture","`+field+`":"11111111-1111-4111-8111-111111111111","max_tokens":10,"messages":[{"role":"user","content":"hello"}]}`))
		if field == "release_id" {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	require.Equal(t, 1, calls)
}
