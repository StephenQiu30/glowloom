package aigateway_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	agent "github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	agentwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/agent"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

func TestAgentModelGatewayReplaysPaidCallWithReducedRunBudget(t *testing.T) {
	db, _, model, actor := fixture(t)
	project := entrypointProject(t, db, actor)
	entrypointSession(t, db, actor)
	var sends atomic.Int64
	runtime := &aigateway.Runtime{DB: db, Provider: providerFunc(func(ctx context.Context, dispatch application.Dispatch, emit func(json.RawMessage) error) (domain.Receipt, error) {
		sends.Add(1)
		return textProvider(ctx, dispatch, emit)
	})}
	boundary := agentwire.NewBoundary(db, runtime.AgentModel).WithGateway(runtime)
	selection, err := json.Marshal(map[string]any{"model_key": model.Name, "release_id": model.ID, "max_tokens": 100})
	require.NoError(t, err)
	scope := agent.Scope{ActorID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, ProjectID: project}
	admission := agent.Admission{
		RootRunID: uuid.New(), SessionID: uuid.New(), SubmissionID: uuid.New(), RequestID: uuid.New(),
		ContentHash: agent.Hash([]byte("model gateway replay")), Scope: scope,
		BudgetMicros: 2000000, ModelSelection: selection,
	}
	require.NoError(t, boundary.Store.AdmitRun(t.Context(), admission))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	const token = "isolated-agent-gateway-service-token"
	boundary.BusinessHandler(token).Register(router)
	callID := uuid.NewString()
	invoke := func(id, input string, stream bool) *httptest.ResponseRecorder {
		t.Helper()
		raw, err := json.Marshal(map[string]any{
			"model": model.Name, "stream": stream,
			"messages": []map[string]string{{"role": "user", "content": input}},
		})
		require.NoError(t, err)
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/internal/agent-gateway/openai/chat/completions", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Agent-Audience", "agent-business")
		req.Header.Set("X-Glowloom-Actor-Id", scope.ActorID.String())
		req.Header.Set("X-Glowloom-Org-Id", scope.OrgID.String())
		req.Header.Set("X-Glowloom-Scope-Id", scope.ScopeID.String())
		req.Header.Set("X-Glowloom-Project-Id", project.String())
		req.Header.Set("X-Glowloom-Run-Id", admission.RootRunID.String())
		req.Header.Set("X-Glowloom-Call-Id", id)
		reply := httptest.NewRecorder()
		router.ServeHTTP(reply, req)
		return reply
	}

	streaming := invoke(callID, "fixture", true)
	require.Equal(t, http.StatusUnprocessableEntity, streaming.Code, streaming.Body.String())
	require.Zero(t, sends.Load())
	first := invoke(callID, "fixture", false)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	require.Contains(t, first.Body.String(), "finished text")
	snapshot, err := boundary.Store.CostSnapshot(t.Context(), admission.RootRunID)
	require.NoError(t, err)
	require.EqualValues(t, 100000, snapshot.SettledCharge)
	require.Zero(t, snapshot.PendingCommitment)
	again := invoke(callID, "fixture", false)
	require.Equal(t, http.StatusOK, again.Code, again.Body.String())
	require.JSONEq(t, first.Body.String(), again.Body.String())
	changed := invoke(callID, "changed input", false)
	require.Equal(t, http.StatusConflict, changed.Code, changed.Body.String())
	malformed := invoke("invalid-call-id", "fixture", false)
	require.Equal(t, http.StatusUnprocessableEntity, malformed.Code, malformed.Body.String())
	require.EqualValues(t, 1, sends.Load())
	var settlements int64
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.money_event WHERE scope_id=? AND kind='settle'", scope.ScopeID).Scan(&settlements).Error)
	require.EqualValues(t, 1, settlements)
}

// Node's production client uses this authenticated HTTP business contract. Quote,
// proposal, user approval, continuation, confirmation and task reads stay real.
func TestAgentBusinessHTTPApprovedQuoteUsesOneNewLedger(t *testing.T) {
	db, _, m, a := fixture(t)
	p := entrypointProject(t, db, a)
	entrypointSession(t, db, a)
	runtime := &aigateway.Runtime{DB: db, Provider: providerFunc(textProvider)}
	boundary := agentwire.NewBoundary(db, runtime.AgentModel).WithGateway(runtime)
	admission := agent.Admission{RootRunID: uuid.New(), SessionID: uuid.New(), SubmissionID: uuid.New(), RequestID: uuid.New(), ContentHash: agent.Hash([]byte("new gateway HTTP")), Scope: agent.Scope{ActorID: a.ID, OrgID: a.OrgID, ScopeID: a.ScopeID, ProjectID: p}, BudgetMicros: 2000000}
	require.NoError(t, boundary.Store.AdmitRun(t.Context(), admission))
	grant, err := boundary.Store.RegisterGrant(t.Context(), agent.GrantRequest{RootRunID: admission.RootRunID, SubmissionID: admission.SubmissionID, OwnerEpoch: 1, AuthorizationRevision: 1, LeaseUntil: time.Now().Add(time.Minute)})
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	const token = "isolated-ai-business-service-token"
	boundary.BusinessHandler(token).Register(router)
	invoke := func(name, key string, input any, maximum int64) (agent.CallReceipt, int) {
		t.Helper()
		raw, e := json.Marshal(input)
		require.NoError(t, e)
		body, e := json.Marshal(agent.CallInput{Grant: grant, Call: agent.CallIdentity{TaskID: "production-client", CallID: key, CheckpointID: "durable", Attempt: 1, ContractVersion: "v1", InputHash: agent.Hash(raw)}, Input: raw, MaxMicros: maximum})
		require.NoError(t, e)
		req := httptest.NewRequestWithContext(t.Context(), "POST", "/internal/agent-business/tools/"+name+"/execute", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Agent-Audience", "agent-business")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		var out agent.CallReceipt
		if w.Code == 200 {
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		} else {
			t.Log(name, w.Code, w.Body.String())
		}
		return out, w.Code
	}
	quoted, status := invoke("operation_quote", "quote", map[string]any{"model_key": m.Name, "capability": "text.generate", "mode": "text_to_text", "prompt": "business contract", "params": map[string]any{"max_tokens": 100}, "output_count": 1}, 0)
	require.Equal(t, 200, status)
	var quote struct {
		OperationID uuid.UUID `json:"operation_id"`
	}
	require.NoError(t, json.Unmarshal(quoted.Result, &quote))
	proposalReceipt, status := invoke("operation_propose", "proposal", map[string]any{"operation_id": quote.OperationID}, 0)
	require.Equal(t, 200, status)
	var proposal agent.Proposal
	require.NoError(t, json.Unmarshal(proposalReceipt.Result, &proposal))
	_, status = invoke("operation_confirm", "before-approval", map[string]any{"operation_id": quote.OperationID}, 1000000)
	require.NotEqual(t, 200, status)
	decision, err := boundary.Store.Decide(t.Context(), a, p, grant.RootRunID, agent.DecisionInput{RequestID: uuid.New(), Action: "approve", ProposalID: proposal.ProposalID, CandidateHash: proposal.CandidateHash, ExpectedAuthorizationRevision: proposal.AuthorizationRevision})
	require.NoError(t, err)
	grant, err = boundary.Store.Quiesce(t.Context(), grant.RootRunID, agent.QuiescedReceipt{ReceiptID: uuid.New(), DecisionID: decision.DecisionID, AuthorizationRevision: decision.AuthorizationRevision, OwnerEpoch: 1, ContinuationID: agent.ContinuationID(grant.RootRunID, decision.DecisionID), LeaseUntil: time.Now().Add(time.Minute)})
	require.NoError(t, err)
	_, status = invoke("operation_confirm", "zero-cap", map[string]any{"operation_id": quote.OperationID}, 0)
	require.NotEqual(t, 200, status)
	confirmed, status := invoke("operation_confirm", "confirm", map[string]any{"operation_id": quote.OperationID}, 1000000)
	require.Equal(t, 200, status)
	again, status := invoke("operation_confirm", "confirm", map[string]any{"operation_id": quote.OperationID}, 1000000)
	require.Equal(t, 200, status)
	require.Equal(t, confirmed.ReceiptID, again.ReceiptID)
	require.JSONEq(t, string(confirmed.Result), string(again.Result))
	var count int64
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.hold WHERE request_id=?", quote.OperationID).Scan(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Raw("SELECT count(*) FROM infra.task_work WHERE kind='ai.execute' AND business_id=?", quote.OperationID.String()).Scan(&count).Error)
	require.EqualValues(t, 1, count)
	snapshot, err := boundary.Store.CostSnapshot(t.Context(), grant.RootRunID)
	require.NoError(t, err)
	require.EqualValues(t, 100000, snapshot.PendingCommitment)
	require.Zero(t, snapshot.SettledCharge)
	creation := aigateway.Creation{Runtime: runtime}
	for range 2 {
		_, err = creation.Execute(t.Context(), taskruntime.Job{BusinessID: quote.OperationID.String()})
		require.NoError(t, err)
	}
	taskReceipt, status := invoke("operation_task", "task", map[string]any{"operation_id": quote.OperationID}, 0)
	require.Equal(t, 200, status)
	var task operation.TaskDetail
	require.NoError(t, json.Unmarshal(taskReceipt.Result, &task))
	require.Equal(t, "completed", string(task.Status))
	require.Equal(t, "finished text", *task.Outputs[0].Text)
	snapshot, err = boundary.Store.CostSnapshot(t.Context(), grant.RootRunID)
	require.NoError(t, err)
	require.Zero(t, snapshot.PendingCommitment)
	require.EqualValues(t, 100000, snapshot.SettledCharge)
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.money_event WHERE request_id=? AND kind='settle'", quote.OperationID).Scan(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Raw("SELECT count(*) FROM billing.reservation WHERE operation_id=?", quote.OperationID).Scan(&count).Error)
	require.Zero(t, count)
}
