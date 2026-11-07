package modelgateway_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/modelgateway"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

func TestChatAllowsBoundedVideoFrameProjection(t *testing.T) {
	calls := 0
	client := modelgateway.NewChatClient(&http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"inspected"}}],"usage":{"prompt_tokens":241,"completion_tokens":1}}`))}, nil
	})})
	config := catalog.ProviderConfig{AdapterKey: "openrouter", BaseURL: "https://openrouter.ai/api/v1", Region: "overseas", Capabilities: []string{"text"}, TimeoutSeconds: 30, ConcurrencyLimit: 1, RateLimitPerMin: 10}
	parts := make([]map[string]any, 241)
	for i := range parts {
		parts[i] = map[string]any{"type": "text", "text": "实际读取的参考内容"}
	}
	request := map[string]any{"model_key": "planner", "max_tokens": 128, "messages": []any{map[string]any{"role": "user", "content": parts}}}
	raw, _ := json.Marshal(request)
	if _, err := client.Complete(t.Context(), config, "fixture", "upstream", raw); err != nil {
		t.Fatal("bounded 30 image/30 video frame projection rejected", err)
	}
	parts = append(parts, make([]map[string]any, 16)...)
	for i := 241; i < len(parts); i++ {
		parts[i] = map[string]any{"type": "text", "text": "bounded"}
	}
	request["messages"] = []any{map[string]any{"role": "user", "content": parts}}
	raw, _ = json.Marshal(request)
	if _, err := client.Complete(t.Context(), config, "fixture", "upstream", raw); err == nil {
		t.Fatal("unbounded content parts accepted")
	}
	if calls != 1 {
		t.Fatal("invalid projection reached provider", calls)
	}
}
