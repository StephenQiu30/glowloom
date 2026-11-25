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
					"args":       json.RawMessage(raw),
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
	unauthReq := httptest.NewRequest(stdhttp.MethodPost, "/internal/agent-mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	unauthRec := httptest.NewRecorder()
	router.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated MCP, got %d", unauthRec.Code)
	}

	// 2. MCP tools/list succeeds with valid identity and standard Streamable HTTP Accept header.
	listReq := httptest.NewRequest(stdhttp.MethodPost, "/internal/agent-mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
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
	callReq := httptest.NewRequest(stdhttp.MethodPost, "/internal/agent-mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"read_project_script","arguments":{"episode":1}}}`))
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

	// 4. Unified channel pass-through gateway /v1/chat/completions succeeds.
	gwReq := httptest.NewRequest(stdhttp.MethodPost, "/internal/agent-gateway/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o","messages":[{"role":"user","content":"plan shot 1"}]}`))
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
}
