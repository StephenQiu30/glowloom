package aigateway_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/provider"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

func nativeModel(protocol string) domain.Model {
	return domain.Model{ID: uuid.New(), Name: "test", Protocol: protocol, Price: domain.Price{Input: 1000000, Output: 1000000, CacheRead: 1000000, CacheWrite: 1000000, Currency: "CNY", FX: "1", Multiplier: "1", Discount: "1"}, MaxInput: 8192, MaxOutput: 1000, PurchaseCap: 100000, Tools: true, Stream: true}
}
func TestNativeToolRoundTripsPreserveMatchingCalls(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "messages"} {
		t.Run(protocol, func(t *testing.T) {
			var body map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(payload(protocol, "test", false)), &body))
			switch protocol {
			case "chat":
				body["messages"] = json.RawMessage(`[{"role":"user","content":"read"},{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"read","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_1","content":"result"}]`)
			case "responses":
				body["input"] = json.RawMessage(`[{"role":"user","content":"read"},{"type":"function_call","call_id":"call_1","name":"read","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"result"}]`)
			case "messages":
				body["messages"] = json.RawMessage(`[{"role":"user","content":"read"},{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"read","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"result"}]}]`)
			}
			raw, err := json.Marshal(body)
			require.NoError(t, err)
			call, err := application.Prepare(nativeModel(protocol), uuid.New(), uuid.New(), uuid.Nil, uuid.Nil, "roundtrip", "test", raw)
			require.NoError(t, err)
			require.Contains(t, string(call.Raw), "call_1")
			altered := strings.ReplaceAll(string(raw), `"result"`, `"result-changed"`)
			other, err := application.Prepare(call.Model, call.Actor, call.Scope, uuid.Nil, uuid.Nil, "roundtrip", "test", json.RawMessage(altered))
			require.NoError(t, err)
			require.NotEqual(t, call.Hash, other.Hash)
			// Removing the required result must reject the incomplete tool turn before funds.
			switch protocol {
			case "responses":
				body["input"] = json.RawMessage(`[{"type":"function_call_output","call_id":"orphan","output":"result"}]`)
			case "chat":
				body["messages"] = json.RawMessage(`[{"role":"tool","tool_call_id":"orphan","content":"result"}]`)
			default:
				body["messages"] = json.RawMessage(`[{"role":"user","content":[{"type":"tool_result","tool_use_id":"orphan","content":"result"}]}]`)
			}
			raw, _ = json.Marshal(body)
			_, err = application.Prepare(call.Model, call.Actor, call.Scope, uuid.Nil, uuid.Nil, "orphan", "test", raw)
			require.ErrorIs(t, err, domain.ErrInvalid)
		})
	}
}
func TestNativeBrokenSSEAndUndeclaredToolsNeverSettle(t *testing.T) {
	cases := []struct{ name, protocol, body string }{
		{"chat-no-finish", "chat", `data: {"model":"actual-supplier","choices":[],"usage":{"prompt_tokens":12,"completion_tokens":3}}` + "\n\ndata: [DONE]\n\n"},
		{"chat-broken-tool", "chat", strings.Replace(response("chat", true, false), `"arguments":"{}"`, `"arguments":"{"`, 1)},
		{"responses-no-output", "responses", "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"model\":\"actual-supplier\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"},
		{"messages-no-block-stop", "messages", strings.Replace(response("messages", true, false), "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n", "", 1)},
		{"messages-incomplete-json", "messages", strings.Replace(response("messages", true, false), `"partial_json":"{}"`, `"partial_json":"{"`, 1)},
		{"wrong-model", "chat", strings.Replace(response("chat", true, false), "actual-supplier", "different-model", 1)},
		{"unknown-tool", "chat", strings.ReplaceAll(response("chat", true, false), `"name":"read"`, `"name":"unregistered"`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := nativeModel(tc.protocol)
			call, err := application.Prepare(m, uuid.New(), uuid.New(), uuid.Nil, uuid.Nil, "broken", "test", json.RawMessage(payload(tc.protocol, "test", true)))
			require.NoError(t, err)
			count := 0
			client := provider.New(keyFixture{}, &http.Client{Transport: transportFunc(func(_ *http.Request) (*http.Response, error) {
				count++
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})})
			d := application.Dispatch{Call: call, Route: domain.Route{Protocol: tc.protocol, BaseURL: "https://8.8.8.8/v1", UpstreamModel: "actual-supplier", Purchase: m.Price}}
			_, err = client.Send(context.Background(), d, nil)
			require.ErrorIs(t, err, domain.ErrUnknown)
			require.Equal(t, 1, count)
		})
	}
}
func TestNativeCacheTTLAndUnsupportedOptionsFailBeforeAdmission(t *testing.T) {
	for _, extra := range []string{`,"service_tier":"priority"`, `,"tool_choice":{"type":"tool","name":"not_declared"}`, `,"temperature":2`} {
		raw := strings.TrimSuffix(payload("messages", "test", false), "}") + extra + "}"
		_, err := application.Prepare(nativeModel("messages"), uuid.New(), uuid.New(), uuid.Nil, uuid.Nil, "invalid", "test", json.RawMessage(raw))
		require.ErrorIs(t, err, domain.ErrInvalid)
	}
	raw := `{"model":"test","max_tokens":100,"messages":[{"role":"user","content":[{"type":"text","text":"cache","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`
	_, err := application.Prepare(nativeModel("messages"), uuid.New(), uuid.New(), uuid.Nil, uuid.Nil, "ttl", "test", json.RawMessage(raw))
	require.ErrorIs(t, err, domain.ErrInvalid)
}
