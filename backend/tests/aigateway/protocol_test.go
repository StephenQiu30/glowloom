package aigateway_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go/v2"
	ao "github.com/anthropics/anthropic-sdk-go/v2/option"
	openai "github.com/openai/openai-go/v3"
	oo "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"

	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	gatewayhttp "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/http"
	pg "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/provider"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
)

type keyFixture struct{}

func (keyFixture) Open(string, uuid.UUID, uuid.UUID, []byte) ([]byte, error) {
	return []byte(`{"api_key":"fixture-only"}`), nil
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func payload(protocol, name string, stream bool) string {
	input := `"messages":[{"role":"user","content":"fixture"}],"max_tokens":100`
	if protocol == "responses" {
		input = `"input":"fixture","max_output_tokens":100`
	}
	tool := `"tools":[{"type":"function","function":{"name":"read","parameters":{"type":"object"}}}]`
	if protocol == "responses" {
		tool = `"tools":[{"type":"function","name":"read","parameters":{"type":"object"}}]`
	}
	if protocol == "messages" {
		tool = `"tools":[{"name":"read","input_schema":{"type":"object"}}]`
	}
	return fmt.Sprintf(`{"model":%q,"stream":%t,%s,%s}`, name, stream, input, tool)
}
func response(protocol string, stream, broken bool) string {
	if !stream {
		switch protocol {
		case "chat":
			return `{"model":"actual-supplier","id":"chat_test","object":"chat.completion","choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"tool_1","type":"function","function":{"name":"read","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":3,"prompt_tokens_details":{"cached_tokens":2}}}`
		case "responses":
			return `{"model":"actual-supplier","id":"resp_test","status":"completed","object":"response","output":[{"type":"function_call","call_id":"tool_1","name":"read","arguments":"{}"}],"usage":{"input_tokens":12,"output_tokens":3,"input_tokens_details":{"cached_tokens":2}}}`
		default:
			return `{"model":"actual-supplier","id":"msg_test","type":"message","role":"assistant","content":[{"type":"tool_use","id":"tool_1","name":"read","input":{}}],"stop_reason":"tool_use","usage":{"input_tokens":7,"cache_read_input_tokens":2,"cache_creation_input_tokens":3,"output_tokens":3}}`
		}
	}
	if broken {
		return "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"
	}
	frame := func(event, data string) string {
		if event != "" {
			return "event: " + event + "\ndata: " + data + "\n\n"
		}
		return "data: " + data + "\n\n"
	}
	switch protocol {
	case "chat":
		return frame("", `{"model":"actual-supplier","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"tool_1","type":"function","function":{"name":"read","arguments":"{}"}}]},"finish_reason":null}]}`) + frame("", `{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`) + frame("", `{"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":3,"prompt_tokens_details":{"cached_tokens":2}}}`) + frame("", "[DONE]")
	case "responses":
		return frame("response.function_call_arguments.delta", `{"type":"response.function_call_arguments.delta","item_id":"tool_1","delta":"{}"}`) + frame("response.completed", `{"type":"response.completed","response":`+response(protocol, false, false)+`}`)
	default:
		return frame("message_start", `{"type":"message_start","message":{"model":"actual-supplier","usage":{"input_tokens":7,"cache_read_input_tokens":2,"cache_creation_input_tokens":3,"output_tokens":0}}}`) + frame("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_1","name":"read","input":{}}}`) + frame("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`) + frame("content_block_stop", `{"type":"content_block_stop","index":0}`) + frame("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":3}}`) + frame("message_stop", `{"type":"message_stop"}`)
	}
}
func TestRebuiltThreeNativeProtocolsAndSSEUseActualLedger(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "messages"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream_%t", protocol, stream), func(t *testing.T) {
				db, store, m, a := fixture(t)
				m.ID = uuid.New()
				m.Name += "-" + protocol
				m.Protocol = protocol
				require.NoError(t, store.PublishModel(t.Context(), m))
				route := domain.Route{ID: uuid.New(), ModelID: m.ID, Supplier: uuid.New(), Credential: uuid.New(), Group: uuid.NewString(), Protocol: protocol, BaseURL: "https://8.8.8.8/v1", UpstreamModel: "actual-supplier", Purchase: m.Price}
				require.NoError(t, store.PublishRoute(t.Context(), route, []byte("fixture"), "fixture", 2, 10000))
				token, err := store.IssueGrant(t.Context(), a.ScopeID, []uuid.UUID{m.ID}, 50, 600)
				require.NoError(t, err)
				calls := 0
				upstream := provider.New(keyFixture{}, &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					var raw map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&raw))
					require.Equal(t, "actual-supplier", raw["model"])
					require.Equal(t, "8.8.8.8", r.URL.Hostname())
					if protocol == "messages" {
						require.Equal(t, "fixture-only", r.Header.Get("X-Api-Key"))
					} else {
						require.Equal(t, "Bearer fixture-only", r.Header.Get("Authorization"))
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(response(protocol, stream, false)))}, nil
				})})
				gin.SetMode(gin.TestMode)
				router := gin.New()
				api := router.Group("/api")
				handler := gatewayhttp.New(db, fixtureAuthority, upstream, nil)
				handler.Register(api, api.Group("/session"))
				path := "chat/completions"
				if protocol == "responses" {
					path = "responses"
				}
				if protocol == "messages" {
					path = "messages"
				}
				var outputs []string
				for i := 0; i < 2; i++ {
					req := httptest.NewRequestWithContext(t.Context(), "POST", "/api/ai/v1/"+path, strings.NewReader(payload(protocol, m.Name, stream)))
					req.Header.Set("Authorization", "Bearer "+token)
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Idempotency-Key", "logical-call")
					w := httptest.NewRecorder()
					router.ServeHTTP(w, req)
					require.Equal(t, 200, w.Code, w.Body.String())
					outputs = append(outputs, w.Body.String())
				}

				server := httptest.NewServer(router)
				defer server.Close()
				for n := 0; n < 2; n++ {
					require.NoError(t, officialClient(t.Context(), t, protocol, stream, server.URL+"/api/ai/v1/", token, payload(protocol, m.Name, stream)))
				}

				require.Equal(t, 2, calls)
				require.Equal(t, outputs[0], outputs[1])
				require.Contains(t, outputs[0], "tool_1")
				grant, err := pg.ResolveGrant(t.Context(), db, token)
				require.NoError(t, err)
				var n int64
				require.NoError(t, db.Raw("SELECT count(*) FROM ai.money_event WHERE scope_id=? AND kind='settle'", grant.Scope).Scan(&n).Error)
				require.EqualValues(t, 2, n)
				bills, err := store.Bills(t.Context(), a.ScopeID)
				require.NoError(t, err)
				require.Len(t, bills, 2)
				require.EqualValues(t, 1, bills[0].Charged)
				require.EqualValues(t, 12, bills[0].Receipt.Usage.Input)
			})
		}
	}
}

func fixtureAuthority(_ context.Context, tx *gorm.DB, a identity.Principal, scope, _ uuid.UUID, admin bool) error {
	var n int64
	if err := tx.Raw(`SELECT count(*) FROM workspace.creation_scope s JOIN identity."user" u ON u.id=s.owner_user_id WHERE s.id=? AND u.id=? AND u.status='active' AND NOT u.is_delete AND (NOT ? OR u.platform_role='platform_admin')`, scope, a.ID, admin).Scan(&n).Error; err != nil {
		return err
	}
	if n != 1 {
		return domain.ErrForbidden
	}
	return nil
}

func officialClient(ctx context.Context, t *testing.T, protocol string, stream bool, base, token, body string) error {
	switch protocol {
	case "chat":
		c := openai.NewClient(oo.WithAPIKey(token), oo.WithBaseURL(base), oo.WithMaxRetries(0))
		var p openai.ChatCompletionNewParams
		if err := json.Unmarshal([]byte(body), &p); err != nil {
			return err
		}
		if stream {
			it := c.Chat.Completions.NewStreaming(ctx, p, oo.WithHeader("Idempotency-Key", "sdk-call"))
			defer func() { require.NoError(t, it.Close()) }()
			for it.Next() {
			}
			return it.Err()
		}
		_, err := c.Chat.Completions.New(ctx, p, oo.WithHeader("Idempotency-Key", "sdk-call"))
		return err
	case "responses":
		c := openai.NewClient(oo.WithAPIKey(token), oo.WithBaseURL(base), oo.WithMaxRetries(0))
		var p responses.ResponseNewParams
		if err := json.Unmarshal([]byte(body), &p); err != nil {
			return err
		}
		if stream {
			it := c.Responses.NewStreaming(ctx, p, oo.WithHeader("Idempotency-Key", "sdk-call"))
			defer func() { require.NoError(t, it.Close()) }()
			for it.Next() {
			}
			return it.Err()
		}
		_, err := c.Responses.New(ctx, p, oo.WithHeader("Idempotency-Key", "sdk-call"))
		return err
	default:
		c := anthropic.NewClient(ao.WithAPIKey(token), ao.WithBaseURL(strings.TrimSuffix(base, "v1/")), ao.WithMaxRetries(0))
		var p anthropic.MessageNewParams
		if err := json.Unmarshal([]byte(body), &p); err != nil {
			return err
		}
		if stream {
			it := c.Messages.NewStreaming(ctx, p, ao.WithHeader("Idempotency-Key", "sdk-call"))
			defer func() { require.NoError(t, it.Close()) }()
			for it.Next() {
			}
			return it.Err()
		}
		_, err := c.Messages.New(ctx, p, ao.WithHeader("Idempotency-Key", "sdk-call"))
		return err
	}
}
