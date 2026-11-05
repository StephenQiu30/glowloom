package operation_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/modelgateway"
	agentpg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	agent "github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billing "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	operationpg "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

func TestModelGatewayPersistsDialogueUsageAndAtomicSettlement(t *testing.T) {
	for _, scenario := range []string{"complete", "response_lost", "settlement_interrupted", "budget_rejected", "grant_revoked", "wrong_release", "params_tampered", "model_tampered", "tokens_tampered", "identity_revoked_after_send", "skill_disabled_before_dispatch", "skill_enabled_before_dispatch"} {
		t.Run(scenario, func(t *testing.T) {
			dsn := os.Getenv("GLOWLOOM_TEST_MODEL_GATEWAY_DB_DSN")
			if dsn == "" {
				t.Skip("set disposable model gateway PostgreSQL database")
			}
			conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
			require.NoError(t, err)
			t.Cleanup(func() {
				if err := conn.Close(); err != nil {
					t.Error("close database connection:", err)
				}
			})
			database := conn.DB
			f := seedManagedFixture(t, database)
			require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET capability='text.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, f.identity.ModelProfileVersionID).Error)
			require.NoError(t, database.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text_to_text'],queue='provider',supports_cancel=false,supports_query=false,moderation='provider' WHERE id=?`, f.identity.ModelProfileVersionID).Error)
			require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET unit='per_request',rule='{"base_micros":10}' WHERE id=?`, f.identity.PriceRuleVersionID).Error)
			var actor struct{ ID, OrgID, ScopeID uuid.UUID }
			require.NoError(t, database.Raw(`SELECT o.confirmed_by AS id,p.org_id,p.owner_scope_id AS scope_id FROM operation.operation o JOIN workspace.project p ON p.id=o.project_id WHERE o.id=?`, f.identity.OperationID).Scan(&actor).Error)
			scope := agent.Scope{ActorID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, ProjectID: f.identity.ProjectID}
			var model string
			require.NoError(t, database.Raw(`SELECT model_key FROM catalog.model_profile m JOIN catalog.model_profile_version v ON v.model_profile_id=m.id WHERE v.id=?`, f.identity.ModelProfileVersionID).Scan(&model).Error)
			input, _ := json.Marshal(map[string]any{"model_key": model, "model_release_id": f.release.String(), "max_tokens": 100, "messages": []map[string]any{{"role": "user", "content": "read the scene"}}, "tools": []map[string]any{{"name": "read_context", "description": "read project", "parameters": map[string]any{"type": "object"}}}})
			if scenario == "wrong_release" || scenario == "params_tampered" || scenario == "model_tampered" || scenario == "tokens_tampered" {
				var value map[string]any
				require.NoError(t, json.Unmarshal(input, &value))
				switch scenario {
				case "wrong_release":
					value["model_release_id"] = uuid.NewString()
				case "params_tampered":
					value["params"] = map[string]any{"temperature": 0.9}
				case "model_tampered":
					value["model_key"] = "unapproved-model"
				case "tokens_tampered":
					value["max_tokens"] = 101
				}
				input, _ = json.Marshal(value)
			}
			rid, segment, id := uuid.New(), uuid.New(), uuid.New()
			scopeRaw, _ := json.Marshal(scope)
			maximum := int64(20)
			if scenario == "budget_rejected" {
				maximum = 1
			}
			require.NoError(t, database.Exec(`INSERT INTO agent_business.run_authorization(root_run_id,session_id,submission_id,request_id,content_hash,scope,budget_micros,reserved_micros,owner_epoch) VALUES(?,?,?,?,?,?::jsonb,100,?,1)`, rid, uuid.New(), segment, uuid.New(), agent.Hash(input), string(scopeRaw), maximum).Error)
			require.NoError(t, database.Exec(`INSERT INTO agent_business.execution_grant(grant_id,root_run_id,submission_id,owner_epoch,authorization_revision,expires_at) VALUES(?,?,?,1,1,clock_timestamp()+interval '1 hour')`, uuid.New(), rid, segment).Error)
			require.NoError(t, database.Exec(`INSERT INTO agent_business.call_receipt(receipt_id,root_run_id,submission_id,task_id,call_id,checkpoint_id,attempt,contract_version,input_hash,kind,name,max_micros,status) VALUES(?,?,?,'task','call','checkpoint',1,'v1',?,'model','model',?,'in_flight')`, id, rid, segment, agent.Hash(input), maximum).Error)
			selection, _ := json.Marshal(map[string]any{"model_key": model, "params": map[string]any{}, "max_tokens": 100})
			require.NoError(t, database.Exec(`UPDATE agent_business.run_authorization SET model_selection=?::jsonb WHERE root_run_id=?`, string(selection), rid).Error)
			if scenario == "skill_disabled_before_dispatch" || scenario == "skill_enabled_before_dispatch" {
				pin := agent.SkillPin{SkillID: uuid.New(), Version: "1", ResourceHash: agent.Hash(json.RawMessage(`{"fixture":true}`))}
				require.NoError(t, database.Exec(`INSERT INTO agent_business.skill_version(skill_id,version,resource_hash,owner_actor_id,org_id,scope_id,name,description,body,enabled) VALUES(?,?,?, ?,?,?,'fixture','fixture','fixture',?)`, pin.SkillID, pin.Version, pin.ResourceHash, scope.ActorID, scope.OrgID, scope.ScopeID, scenario == "skill_enabled_before_dispatch").Error)
				pins, _ := json.Marshal([]agent.SkillPin{pin})
				require.NoError(t, database.Exec(`UPDATE agent_business.run_authorization SET skill_pins=?::jsonb WHERE root_run_id=?`, string(pins), rid).Error)
			}
			if scenario == "grant_revoked" {
				require.NoError(t, database.Exec(`UPDATE agent_business.run_authorization SET barrier=true WHERE root_run_id=?`, rid).Error)
			}
			if scenario == "settlement_interrupted" {
				require.NoError(t, database.Exec(`CREATE TRIGGER gateway_fail_settle BEFORE INSERT ON billing.ledger_entry FOR EACH ROW WHEN (NEW.entry_type='settle') EXECUTE FUNCTION pg_catalog.suppress_redundant_updates_trigger()`).Error)
				defer database.Exec(`DROP TRIGGER IF EXISTS gateway_fail_settle ON billing.ledger_entry`)
			}
			sends := 0
			client := &http.Client{Transport: adapterTransport(func(r *http.Request) (*http.Response, error) {
				sends++
				require.Equal(t, "Bearer fixture-only-not-real", r.Header.Get("Authorization"))
				if scenario == "response_lost" {
					return nil, errors.New("connection interrupted after POST")
				}
				if scenario == "identity_revoked_after_send" {
					require.NoError(t, database.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, scope.ActorID).Error)
				}
				return adapterResponse(200, `{"choices":[{"finish_reason":"tool_calls","message":{"content":null,"tool_calls":[{"id":"tool-id","type":"function","function":{"name":"read_context","arguments":"{}"}}]}}],"usage":{"prompt_tokens":7,"completion_tokens":3,"prompt_tokens_details":{"cached_tokens":2}}}`), nil
			})}
			operations := operationpg.NewScopedStore(database, operationWorkerAccess, operationWorkerAccess, operationPlatformAccess)
			bills := billingpg.NewScopedStore(database, func(tx *gorm.DB) billing.ProjectAccess { return operationWorkerAccess(tx).(billing.ProjectAccess) })
			gateway := modelgateway.New(database, operations, bills, fixtureUnsealer{}, client)
			result, err := gateway.Model(t.Context(), scope, id, input, maximum)
			if scenario == "budget_rejected" || scenario == "grant_revoked" || scenario == "wrong_release" || scenario == "params_tampered" || scenario == "model_tampered" || scenario == "tokens_tampered" || scenario == "skill_disabled_before_dispatch" {
				require.NoError(t, err)
				require.Equal(t, "failed", result.Status)
				require.Zero(t, sends)
				var n int64
				require.NoError(t, database.Raw(`SELECT count(*) FROM billing.reservation WHERE operation_id=?`, id).Scan(&n).Error)
				require.Zero(t, n)
				return
			}
			if scenario == "response_lost" {
				duplicate, duplicateErr := gateway.Model(t.Context(), scope, id, input, maximum)
				require.NoError(t, duplicateErr)
				require.Equal(t, "unknown", duplicate.Status)
				require.NoError(t, err)
				require.Equal(t, "unknown", result.Status)
				for n := 0; n < 2; n++ {
					pending, recoverErr := gateway.Recover(t.Context(), id)
					require.NoError(t, recoverErr)
					require.Equal(t, "unknown", pending.Status)
				}
				require.Equal(t, 1, sends)
				var status string
				require.NoError(t, database.Raw(`SELECT status FROM billing.reservation WHERE operation_id=?`, id).Scan(&status).Error)
				require.Equal(t, "held", status)
				return
			}
			if scenario == "settlement_interrupted" {
				require.Error(t, err)
				require.Equal(t, "unknown", result.Status)
				require.NoError(t, database.Exec(`DROP TRIGGER gateway_fail_settle ON billing.ledger_entry`).Error)
				result, err = gateway.Recover(t.Context(), id)
			}
			require.NoError(t, err)
			expectedStatus := "completed"
			if scenario == "identity_revoked_after_send" {
				expectedStatus = "failed"
			}
			require.Equal(t, expectedStatus, result.Status)
			require.EqualValues(t, 10, result.CostMicros)
			if scenario == "identity_revoked_after_send" {
				require.Contains(t, string(result.Result), "execution_authority_revoked")
			} else {
				require.Contains(t, string(result.Result), `"tool_calls"`)
				require.Contains(t, string(result.Result), `"cache_read_tokens":2`)
			}
			replay, err := gateway.Recover(t.Context(), id)
			require.NoError(t, err)
			require.Equal(t, result, replay)
			require.Equal(t, 1, sends)
			var row struct {
				Status                                  string
				CostMicros, ReservedMicros, Settlements int64
			}
			require.NoError(t, database.Raw(`SELECT c.status,c.cost_micros,a.reserved_micros,(SELECT count(*) FROM billing.ledger_entry WHERE operation_id=c.receipt_id AND entry_type='settle') AS settlements FROM agent_business.call_receipt c JOIN agent_business.run_authorization a ON a.root_run_id=c.root_run_id WHERE c.receipt_id=?`, id).Scan(&row).Error)
			require.Equal(t, expectedStatus, row.Status)
			require.EqualValues(t, 10, row.CostMicros)
			require.EqualValues(t, 10, row.ReservedMicros)
			require.EqualValues(t, 1, row.Settlements)
			loaded, loadErr := operations.LoadWorkflowOperation(t.Context(), id)
			if scenario == "identity_revoked_after_send" {
				require.Error(t, loadErr)
			} else {
				require.NoError(t, loadErr)
				require.NoError(t, loaded.Operation.Validate())
			}
			// The outer ledger's replay is identical and does not debit root allocation again.
			require.NoError(t, database.Transaction(func(tx *gorm.DB) error { _, err := agentpg.CompleteCall(tx, id, result); return err }))
		})
	}
}
