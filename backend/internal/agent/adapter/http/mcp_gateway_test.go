package http

import (
	"bytes"
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
)

func TestInternalMCPAndGatewayEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	token := strings.Repeat("b", 32)
	caps := app.Capabilities{
		Tools: []app.Tool{{
			Descriptor: app.ToolDescriptor{
				Name:        "read_project_script",
				Description: "Reads the current project script",
				Effect:      "read",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"episode":{"type":"number"}}}`),
			},
			Execute: func(_ context.Context, scope domain.Scope, _ uuid.UUID, raw json.RawMessage) (domain.CallResult, error) {
				out, _ := json.Marshal(map[string]any{
					"project_id": scope.ProjectID.String(),
					"args":       raw,
				})
				return domain.CallResult{Status: domain.CallStatusCompleted, Result: out}, nil
			},
		}},
		Model: func(_ context.Context, _ domain.Scope, _ uuid.UUID, _ json.RawMessage, _ int64) (domain.CallResult, error) {
			out := json.RawMessage(`{"output":"storyboard ready","model_key":"gpt-4o","stop_reason":"stop","usage":{"input_tokens":12,"output_tokens":5}}`)
			return domain.CallResult{Status: domain.CallStatusCompleted, Result: out}, nil
		},
	}

	router := gin.New()
	NewBusinessHandler(nil, caps, token).Register(router)

	// 1. MCP tools/list requires auth.
	unauthReq := httptest.NewRequestWithContext(t.Context(), stdhttp.MethodPost, "/internal/agent-mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	unauthRec := httptest.NewRecorder()
	router.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated MCP, got %d", unauthRec.Code)
	}

	// 2. MCP tools/list succeeds with valid identity and standard Streamable HTTP Accept header.
	listReq := httptest.NewRequestWithContext(t.Context(), stdhttp.MethodPost, "/internal/agent-mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	listReq.Header.Set("Authorization", "Bearer "+token)
	listReq.Header.Set("X-Agent-Audience", "agent-business")
	listReq.Header.Set("Content-Type", "application/json")
	listReq.Header.Set("Accept", "application/json, text/event-stream")
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != stdhttp.StatusOK || !strings.Contains(listRec.Body.String(), "read_project_script") || !strings.Contains(listRec.Body.String(), `"readOnlyHint":true`) {
		t.Fatalf("unexpected MCP tools/list response: %d %s", listRec.Code, listRec.Body.String())
	}

	// 3. MCP tools/call invokes business tool with scoped identity.
	actorID, orgID, scopeID, projectID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	callReq := httptest.NewRequestWithContext(t.Context(), stdhttp.MethodPost, "/internal/agent-mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"read_project_script","arguments":{"episode":1}}}`))
	callReq.Header.Set("Authorization", "Bearer "+token)
	callReq.Header.Set("X-Agent-Audience", "agent-business")
	callReq.Header.Set("Content-Type", "application/json")
	callReq.Header.Set("Accept", "application/json, text/event-stream")
	callReq.Header.Set("X-Glowloom-Actor-Id", actorID.String())
	callReq.Header.Set("X-Glowloom-Org-Id", orgID.String())
	callReq.Header.Set("X-Glowloom-Scope-Id", scopeID.String())
	callReq.Header.Set("X-Glowloom-Project-Id", projectID.String())
	callRec := httptest.NewRecorder()
	router.ServeHTTP(callRec, callReq)
	if callRec.Code != stdhttp.StatusOK || !strings.Contains(callRec.Body.String(), projectID.String()) {
		t.Fatalf("unexpected MCP tools/call response: %d %s", callRec.Code, callRec.Body.String())
	}

	// 4. Unified channel pass-through gateway /chat/completions succeeds.
	gwReq := httptest.NewRequestWithContext(t.Context(), stdhttp.MethodPost, "/internal/agent-gateway/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o","messages":[{"role":"user","content":"plan shot 1"}]}`))
	gwReq.Header.Set("Authorization", "Bearer "+token)
	gwReq.Header.Set("X-Agent-Audience", "agent-business")
	gwReq.Header.Set("X-Glowloom-Actor-Id", actorID.String())
	gwReq.Header.Set("X-Glowloom-Org-Id", orgID.String())
	gwReq.Header.Set("X-Glowloom-Scope-Id", scopeID.String())
	gwReq.Header.Set("X-Glowloom-Project-Id", projectID.String())
	gwRec := httptest.NewRecorder()
	router.ServeHTTP(gwRec, gwReq)
	if gwRec.Code != stdhttp.StatusOK || !strings.Contains(gwRec.Body.String(), "storyboard ready") {
		t.Fatalf("unexpected gateway response: %d %s", gwRec.Code, gwRec.Body.String())
	}

	// 5. Multi-protocol OpenAI route alias /openai/chat/completions succeeds.
	openaiReq := httptest.NewRequestWithContext(t.Context(), stdhttp.MethodPost, "/internal/agent-gateway/openai/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o","messages":[{"role":"user","content":"plan shot 2"}]}`))
	openaiReq.Header.Set("Authorization", "Bearer "+token)
	openaiReq.Header.Set("X-Agent-Audience", "agent-business")
	openaiReq.Header.Set("X-Glowloom-Actor-Id", actorID.String())
	openaiReq.Header.Set("X-Glowloom-Org-Id", orgID.String())
	openaiReq.Header.Set("X-Glowloom-Scope-Id", scopeID.String())
	openaiReq.Header.Set("X-Glowloom-Project-Id", projectID.String())
	openaiRec := httptest.NewRecorder()
	router.ServeHTTP(openaiRec, openaiReq)
	if openaiRec.Code != stdhttp.StatusOK || !strings.Contains(openaiRec.Body.String(), "storyboard ready") {
		t.Fatalf("unexpected openai alias gateway response: %d %s", openaiRec.Code, openaiRec.Body.String())
	}

	// 6. OpenAI Responses API /openai/responses succeeds.
	respReq := httptest.NewRequestWithContext(t.Context(), stdhttp.MethodPost, "/internal/agent-gateway/openai/responses", bytes.NewBufferString(`{"model":"gpt-5","input":[{"role":"user","content":"plan shot 3"}]}`))
	respReq.Header.Set("Authorization", "Bearer "+token)
	respReq.Header.Set("X-Agent-Audience", "agent-business")
	respReq.Header.Set("X-Glowloom-Actor-Id", actorID.String())
	respReq.Header.Set("X-Glowloom-Org-Id", orgID.String())
	respReq.Header.Set("X-Glowloom-Scope-Id", scopeID.String())
	respReq.Header.Set("X-Glowloom-Project-Id", projectID.String())
	respRec := httptest.NewRecorder()
	router.ServeHTTP(respRec, respReq)
	if respRec.Code != stdhttp.StatusOK || !strings.Contains(respRec.Body.String(), `"object":"response"`) || !strings.Contains(respRec.Body.String(), "storyboard ready") {
		t.Fatalf("unexpected openai responses gateway response: %d %s", respRec.Code, respRec.Body.String())
	}

	// 7. Anthropic Messages API /anthropic/messages preserves native response format.
	anthReq := httptest.NewRequestWithContext(t.Context(), stdhttp.MethodPost, "/internal/agent-gateway/anthropic/messages", bytes.NewBufferString(`{"model":"claude-sonnet-4-5","max_tokens":1024,"metadata":{"user_id":"sess-1"},"messages":[{"role":"user","content":"plan shot 4"}]}`))
	anthReq.Header.Set("Authorization", "Bearer "+token)
	anthReq.Header.Set("X-Agent-Audience", "agent-business")
	anthReq.Header.Set("X-Glowloom-Actor-Id", actorID.String())
	anthReq.Header.Set("X-Glowloom-Org-Id", orgID.String())
	anthReq.Header.Set("X-Glowloom-Scope-Id", scopeID.String())
	anthReq.Header.Set("X-Glowloom-Project-Id", projectID.String())
	anthRec := httptest.NewRecorder()
	router.ServeHTTP(anthRec, anthReq)
	if anthRec.Code != stdhttp.StatusOK || !strings.Contains(anthRec.Body.String(), `"type":"message"`) || !strings.Contains(anthRec.Body.String(), "storyboard ready") {
		t.Fatalf("unexpected anthropic gateway response: %d %s", anthRec.Code, anthRec.Body.String())
	}

	// 8. Google Gemini /google/models/gemini-2.5-pro:generateContent preserves native candidates format.
	gemReq := httptest.NewRequestWithContext(t.Context(), stdhttp.MethodPost, "/internal/agent-gateway/google/models/gemini-2.5-pro:generateContent", bytes.NewBufferString(`{"contents":[{"role":"user","parts":[{"text":"plan shot 5"}]}]}`))
	gemReq.Header.Set("Authorization", "Bearer "+token)
	gemReq.Header.Set("X-Agent-Audience", "agent-business")
	gemReq.Header.Set("X-Glowloom-Actor-Id", actorID.String())
	gemReq.Header.Set("X-Glowloom-Org-Id", orgID.String())
	gemReq.Header.Set("X-Glowloom-Scope-Id", scopeID.String())
	gemReq.Header.Set("X-Glowloom-Project-Id", projectID.String())
	gemRec := httptest.NewRecorder()
	router.ServeHTTP(gemRec, gemReq)
	if gemRec.Code != stdhttp.StatusOK || !strings.Contains(gemRec.Body.String(), `"candidates"`) || !strings.Contains(gemRec.Body.String(), "storyboard ready") {
		t.Fatalf("unexpected gemini gateway response: %d %s", gemRec.Code, gemRec.Body.String())
	}
}

func TestHarnessMCPTaskWaitAndOperationID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	token := strings.Repeat("c", 32)
	polls := 0
	var capturedCallIDs []uuid.UUID

	caps := app.Capabilities{
		Tools: []app.Tool{
			{
				Descriptor: app.ToolDescriptor{
					Name:   "media_submit_image",
					Effect: "billable",
				},
				Execute: func(_ context.Context, _ domain.Scope, callID uuid.UUID, _ json.RawMessage) (domain.CallResult, error) {
					capturedCallIDs = append(capturedCallIDs, callID)
					return domain.CallResult{
						Status: domain.CallStatusCompleted,
						Result: json.RawMessage(`{"status":"running","taskId":"task-99"}`),
					}, nil
				},
			},
			{
				Descriptor: app.ToolDescriptor{
					Name:   "media_get_task",
					Effect: "read",
				},
				Execute: func(_ context.Context, _ domain.Scope, _ uuid.UUID, _ json.RawMessage) (domain.CallResult, error) {
					polls++
					status := "running"
					if polls >= 2 {
						status = "completed"
					}
					out, _ := json.Marshal(map[string]any{"id": "task-99", "status": status})
					return domain.CallResult{Status: domain.CallStatusCompleted, Result: out}, nil
				},
			},
		},
	}

	router := gin.New()
	NewBusinessHandler(nil, caps, token).Register(router)
	actorID, orgID, scopeID, projectID := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	callMCP := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), stdhttp.MethodPost, "/internal/agent-mcp", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Agent-Audience", "agent-business")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("X-Glowloom-Actor-Id", actorID.String())
		req.Header.Set("X-Glowloom-Org-Id", orgID.String())
		req.Header.Set("X-Glowloom-Scope-Id", scopeID.String())
		req.Header.Set("X-Glowloom-Project-Id", projectID.String())
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// Deterministic operationId yields identical callID across crash replays.
	callMCP(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"media_submit_image","arguments":{"operationId":"op_abc123","prompt":"neon"}}}`)
	callMCP(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"media_submit_image","arguments":{"operationId":"op_abc123","prompt":"neon"}}}`)
	if len(capturedCallIDs) != 2 || capturedCallIDs[0] != capturedCallIDs[1] {
		t.Fatalf("expected deterministic identical callID for replayed operationId, got %v", capturedCallIDs)
	}

	// Built-in task_wait long-polls until task reaches completed.
	waitRec := callMCP(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"task_wait","arguments":{"taskIds":["task-99"],"timeoutMs":400}}}`)
	if waitRec.Code != stdhttp.StatusOK || !strings.Contains(waitRec.Body.String(), "completed") || polls < 2 {
		t.Fatalf("expected task_wait to complete after polling, code=%d polls=%d body=%s", waitRec.Code, polls, waitRec.Body.String())
	}
}
