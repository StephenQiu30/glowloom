package agentbusiness_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	agenthttp "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/http"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
)

func TestWorkflowModelGatewayRespectsFrozenSelectionAndPreservesToolCalls(t *testing.T) {
	store, admission, _ := fixture(t)
	admission.RootRunID = uuid.New()
	admission.SubmissionID = uuid.New()
	admission.RequestID = uuid.New()
	admission.ModelSelection = json.RawMessage(`{"model_key":"test-model","release_id":"c9d0c92d-a0c2-4c42-852f-47e02f61f563","max_tokens":256,"params":{}}`)
	if err := store.AdmitRun(t.Context(), admission); err != nil {
		t.Fatal(err)
	}

	token := strings.Repeat("s", 32)
	router := gin.New()
	calls := 0
	agenthttp.NewBusinessHandler(store, app.Capabilities{Model: func(_ context.Context, scope domain.Scope, _ uuid.UUID, raw json.RawMessage, _ int64) (domain.CallResult, error) {
		calls++
		var input struct {
			Model     string            `json:"model_key"`
			MaxTokens int               `json:"max_tokens"`
			Tools     []json.RawMessage `json:"tools"`
		}
		if err := json.Unmarshal(raw, &input); err != nil || input.Model != "test-model" || input.MaxTokens != 256 || len(input.Tools) != 1 || scope != admission.Scope {
			t.Errorf("unexpected frozen gateway input: %s", raw)
		}
		return domain.CallResult{
			Status: domain.CallStatusCompleted,
			Result: json.RawMessage(`{"tool_calls":[{"id":"call-1","name":"skill_read","arguments":{"path":"references/input.md"}}]}`),
		}, nil
	}}, token).Register(router)

	call := func(project uuid.UUID) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/internal/agent-gateway/chat/completions", strings.NewReader(`{"model":"test-model","messages":[{"role":"user","content":"read the reference"}],"tools":[{"type":"function","function":{"name":"skill_read","parameters":{}}}],"max_tokens":2048}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Agent-Audience", "agent-business")
		req.Header.Set("X-Glowloom-Actor-Id", admission.Scope.ActorID.String())
		req.Header.Set("X-Glowloom-Org-Id", admission.Scope.OrgID.String())
		req.Header.Set("X-Glowloom-Scope-Id", admission.Scope.ScopeID.String())
		req.Header.Set("X-Glowloom-Project-Id", project.String())
		req.Header.Set("X-Glowloom-Run-Id", admission.RootRunID.String())
		req.Header.Set("X-Glowloom-Call-Id", uuid.NewString())
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	if denied := call(uuid.New()); denied.Code < 400 || calls != 0 {
		t.Fatalf("cross-project request reached the model: %d", denied.Code)
	}
	accepted := call(admission.Scope.ProjectID)
	if accepted.Code != http.StatusOK || calls != 1 || !strings.Contains(accepted.Body.String(), `"finish_reason":"tool_calls"`) || !strings.Contains(accepted.Body.String(), `"name":"skill_read"`) {
		t.Fatalf("unexpected model gateway response: %d %s", accepted.Code, accepted.Body)
	}
}
