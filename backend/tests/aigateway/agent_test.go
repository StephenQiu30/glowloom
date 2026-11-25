package aigateway_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	agentpg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	agent "github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

func TestAgentThreeProtocolsToolHistoryOriginalRecoveryAndSingleBudgetProjection(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "messages"} {
		t.Run(protocol, func(t *testing.T) {
			db, store, m, a := fixture(t)
			require.NoError(t, db.Exec(`UPDATE identity."user" SET email_key=?,email_verified_at=clock_timestamp() WHERE id=?`, a.ID.String()+"@example.invalid", a.ID).Error)
			m.ID = uuid.New()
			m.Protocol = protocol
			m.Name = "agent-" + uuid.NewString()
			require.NoError(t, store.PublishModel(t.Context(), m))
			route := domain.Route{ID: uuid.New(), ModelID: m.ID, Supplier: uuid.New(), Credential: uuid.New(), Group: uuid.NewString(), Protocol: protocol, BaseURL: "https://8.8.8.8/v1", UpstreamModel: "native", Purchase: m.Price}
			require.NoError(t, store.PublishRoute(t.Context(), route, []byte("synthetic"), "fixture", 2, 1000))
			pid, root, segment, id := uuid.New(), uuid.New(), uuid.New(), uuid.New()
			require.NoError(t, db.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'Agent rebuilt','16:9','realistic')`, pid, a.OrgID, a.ID, a.ScopeID).Error)
			scope := agent.Scope{ActorID: a.ID, OrgID: a.OrgID, ScopeID: a.ScopeID, ProjectID: pid}
			scopeRaw, _ := json.Marshal(scope)
			input, _ := json.Marshal(map[string]any{"model_key": m.Name, "release_id": m.ID.String(), "max_tokens": 100, "tools": []any{map[string]any{"name": "lookup", "parameters": json.RawMessage(`{"type":"object"}`)}}, "messages": json.RawMessage(`[{"role":"user","content":"query"},{"role":"assistant","content":null,"tool_calls":[{"id":"prior","type":"function","function":{"name":"lookup","arguments":"{\"text\":\"tool_result\"}"}}]},{"role":"tool","tool_call_id":"prior","content":"original tool result"}]`)})
			selection, _ := json.Marshal(map[string]any{"model_key": m.Name, "release_id": m.ID.String(), "max_tokens": 100})
			const maximum = int64(1000000)
			require.NoError(t, db.Exec(`INSERT INTO agent_business.run_authorization(root_run_id,session_id,submission_id,request_id,content_hash,scope,budget_micros,owner_epoch,model_selection) VALUES(?,?,?,?,?,?::jsonb,2000000,1,?::jsonb)`, root, uuid.New(), segment, uuid.New(), agent.Hash(input), string(scopeRaw), string(selection)).Error)
			require.NoError(t, db.Exec(`INSERT INTO agent_business.execution_grant(grant_id,root_run_id,submission_id,owner_epoch,authorization_revision,expires_at) VALUES(?,?,?,1,1,clock_timestamp()+interval '1 hour')`, uuid.New(), root, segment).Error)
			require.NoError(t, db.Exec(`INSERT INTO agent_business.call_receipt(receipt_id,root_run_id,submission_id,task_id,call_id,checkpoint_id,attempt,contract_version,input_hash,kind,name,max_micros,status) VALUES(?,?,?,'task','call','checkpoint',1,'v1',?,'model','model',?,'in_flight')`, id, root, segment, agent.Hash(input), maximum).Error)
			var sends atomic.Int64
			var settlementFault atomic.Bool
			injected := errors.New("isolated settlement interruption")
			callback := "rebuilt-agent-settlement-" + id.String()
			require.NoError(t, db.Callback().Raw().Before("gorm:raw").Register(callback, func(tx *gorm.DB) {
				if settlementFault.Load() && strings.HasPrefix(tx.Statement.SQL.String(), "UPDATE ai.account") {
					_ = tx.AddError(injected)
				}
			}))
			t.Cleanup(func() { require.NoError(t, db.Callback().Raw().Remove(callback)) })
			runtime := &aigateway.Runtime{DB: db, Provider: providerFunc(func(_ context.Context, d application.Dispatch, _ func(json.RawMessage) error) (domain.Receipt, error) {
				sends.Add(1)
				require.Contains(t, string(d.Call.Raw), "original tool result")
				require.Contains(t, string(d.Call.Raw), "tool_result")
				var body string
				switch protocol {
				case "chat":
					body = `{"choices":[{"message":{"content":null,"tool_calls":[{"id":"next","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`
				case "responses":
					body = `{"status":"completed","output":[{"type":"function_call","call_id":"next","name":"lookup","arguments":"{}"}]}`
				default:
					body = `{"stop_reason":"tool_use","content":[{"type":"tool_use","id":"next","name":"lookup","input":{}}]}`
				}
				usage := domain.Usage{Input: 10, Output: 2}
				cost, err := d.Route.Purchase.Cost(usage)
				require.NoError(t, err)
				settlementFault.Store(true)
				return domain.Receipt{Body: json.RawMessage(body), Usage: usage, PurchaseMicros: cost}, nil
			})}
			result, err := runtime.AgentModel(t.Context(), scope, id, input, maximum)
			require.NoError(t, err)
			require.Equal(t, agent.CallStatusUnknown, result.Status, string(result.Result))
			require.EqualValues(t, 1, sends.Load())
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error { _, err := agentpg.CompleteCall(tx, id, result); return err }))
			settlementFault.Store(false)
			recovered, err := runtime.AgentRecover(t.Context(), id)
			require.NoError(t, err)
			require.Equal(t, agent.CallStatusCompleted, recovered.Status)
			require.EqualValues(t, 100000, recovered.CostMicros)
			require.Contains(t, string(recovered.Result), `"id":"next"`)
			require.Contains(t, string(recovered.Result), `"stop_reason":"tool_use"`)
			for range 2 {
				require.NoError(t, db.Transaction(func(tx *gorm.DB) error { _, err := agentpg.CompleteCall(tx, id, recovered); return err }))
			}
			var reserved int64
			require.NoError(t, db.Raw("SELECT reserved_micros FROM agent_business.run_authorization WHERE root_run_id=?", root).Scan(&reserved).Error)
			require.EqualValues(t, 100000, reserved)
			var money struct{ Available, Held, Spent int64 }
			require.NoError(t, db.Raw("SELECT available,held,(SELECT sum(spent_delta) FROM ai.money_event WHERE scope_id=?) AS spent FROM ai.account WHERE scope_id=?", a.ScopeID, a.ScopeID).Scan(&money).Error)
			require.EqualValues(t, 99, money.Available)
			require.Zero(t, money.Held)
			require.EqualValues(t, 1, money.Spent)
			require.EqualValues(t, 1, sends.Load())
		})
	}
}
