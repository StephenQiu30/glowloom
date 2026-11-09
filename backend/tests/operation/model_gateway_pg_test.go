package operation_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/modelgateway"
	agentpg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	agent "github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billing "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	catalogpg "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/postgres"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	operationpg "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	op "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

func TestModelGatewayPersistsDialogueUsageAndAtomicSettlement(t *testing.T) {
	for _, scenario := range []string{"complete", "thinking_explicit", "thinking_default", "thinking_enabled", "thinking_auto", "max_tokens_bound", "max_tokens_exceeded", "runtime_role", "unverified_mode", "credential_rotated", "credential_invalid", "config_changed", "price_revoked", "procurement_revoked", "availability_dependency_missing", "token_pricing", "response_lost", "response_lost_credential_changed", "settlement_interrupted", "budget_rejected", "grant_revoked", "wrong_release", "params_tampered", "model_tampered", "tokens_tampered", "identity_revoked_after_send", "skill_disabled_before_dispatch", "skill_enabled_before_dispatch"} {
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
			seedFixtureProcurement(t, database, f.identity.PriceRuleVersionID, f.identity.ModelProfileVersionID)
			fundOperationProject(t, database, f.identity.ProjectID)
			schema := json.RawMessage(`[{"field":"max_tokens","label":"最大输出","type":"integer","component":"input","min":1,"max":1000}]`)
			if strings.HasPrefix(scenario, "thinking_") || strings.HasPrefix(scenario, "max_tokens_") {
				schema = json.RawMessage(`[{"field":"max_tokens","label":"最大输出","type":"integer","component":"input","min":1,"max":32768,"default":4096},{"field":"thinking","label":"思考模式","type":"string","component":"select","enum":["disabled"],"default":"disabled"}]`)
				require.NoError(t, database.Exec(`UPDATE catalog.provider_config_version SET preset_version_id='6da07a11-0000-4000-8000-000000000001',config=(SELECT jsonb_set(payload->'config','{capabilities}','["text"]') FROM catalog.provider_preset_version WHERE id='6da07a11-0000-4000-8000-000000000001') WHERE id=?`, f.config).Error)
				require.NoError(t, database.Exec(`UPDATE catalog.provider SET adapter_key='volcengine_ark',region='domestic' WHERE id=?`, f.provider).Error)
			}
			if strings.HasPrefix(scenario, "max_tokens_") {
				schema = json.RawMessage(`[{"field":"max_tokens","label":"最大输出","type":"integer","component":"input","min":1,"max":32768,"default":4096}]`)
			}
			seedGatewayAvailability(t, database, f, schema)
			require.NoError(t, database.Exec(`UPDATE billing.budget SET limit_micros=2000000 WHERE project_id=?`, f.identity.ProjectID).Error)
			if scenario == "token_pricing" {
				require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET unit='per_1k_tokens',rule='{"input_micros_per_1k":1000000,"output_micros_per_1k":1000000}' WHERE id::text=(SELECT rule->>'procurement_price_version_id' FROM catalog.price_rule_version WHERE id=?)`, f.identity.PriceRuleVersionID).Error)
				require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET unit='per_1k_tokens',rule=(rule-'base_micros') || '{"input_micros_per_1k":1000000,"output_micros_per_1k":1000000}'::jsonb WHERE id=?`, f.identity.PriceRuleVersionID).Error)
			}
			var actor struct{ ID, OrgID, ScopeID uuid.UUID }
			require.NoError(t, database.Raw(`SELECT o.confirmed_by AS id,p.org_id,p.owner_scope_id AS scope_id FROM operation.operation o JOIN workspace.project p ON p.id=o.project_id WHERE o.id=?`, f.identity.OperationID).Scan(&actor).Error)
			scope := agent.Scope{ActorID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, ProjectID: f.identity.ProjectID}
			var model string
			require.NoError(t, database.Raw(`SELECT model_key FROM catalog.model_profile m JOIN catalog.model_profile_version v ON v.model_profile_id=m.id WHERE v.id=?`, f.identity.ModelProfileVersionID).Scan(&model).Error)
			tokens := 100
			if scenario == "max_tokens_bound" {
				tokens = 32768
			}
			if scenario == "max_tokens_exceeded" {
				tokens = 65536
			}
			requestedParams := map[string]any{}
			switch scenario {
			case "thinking_explicit":
				requestedParams["thinking"] = "disabled"
			case "thinking_enabled":
				requestedParams["thinking"] = "enabled"
			case "thinking_auto":
				requestedParams["thinking"] = "auto"
			}
			input, _ := json.Marshal(map[string]any{"model_key": model, "release_id": f.release.String(), "max_tokens": tokens, "params": requestedParams, "messages": []map[string]any{{"role": "user", "content": "read the scene"}}, "tools": []map[string]any{{"name": "read_context", "description": "read project", "parameters": map[string]any{"type": "object"}}}})
			if scenario == "wrong_release" || scenario == "params_tampered" || scenario == "model_tampered" || scenario == "tokens_tampered" {
				var value map[string]any
				require.NoError(t, json.Unmarshal(input, &value))
				switch scenario {
				case "wrong_release":
					value["release_id"] = uuid.NewString()
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
			maximum := int64(1000000)
			if scenario == "budget_rejected" {
				maximum = 1
			}
			require.NoError(t, database.Exec(`INSERT INTO agent_business.run_authorization(root_run_id,session_id,submission_id,request_id,content_hash,scope,budget_micros,reserved_micros,owner_epoch) VALUES(?,?,?,?,?,?::jsonb,2000000,?,1)`, rid, uuid.New(), segment, uuid.New(), agent.Hash(input), string(scopeRaw), maximum).Error)
			require.NoError(t, database.Exec(`INSERT INTO agent_business.execution_grant(grant_id,root_run_id,submission_id,owner_epoch,authorization_revision,expires_at) VALUES(?,?,?,1,1,clock_timestamp()+interval '1 hour')`, uuid.New(), rid, segment).Error)
			require.NoError(t, database.Exec(`INSERT INTO agent_business.call_receipt(receipt_id,root_run_id,submission_id,task_id,call_id,checkpoint_id,attempt,contract_version,input_hash,kind,name,max_micros,status) VALUES(?,?,?,'task','call','checkpoint',1,'v1',?,'model','model',?,'in_flight')`, id, rid, segment, agent.Hash(input), maximum).Error)
			selection, _ := json.Marshal(map[string]any{"model_key": model, "release_id": f.release.String(), "params": requestedParams, "max_tokens": tokens})
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
			switch scenario {
			case "unverified_mode":
				require.NoError(t, database.Exec(`UPDATE catalog.model_validation SET status='failed' WHERE release_id=?`, f.release).Error)
			case "credential_invalid":
				require.NoError(t, database.Exec(`UPDATE catalog.provider_credential SET last_test_result='auth_failed' WHERE id=?`, f.credential).Error)
			case "config_changed":
				next := uuid.New()
				require.NoError(t, database.Exec(`INSERT INTO catalog.provider_config_version(id,provider_id,preset_version_id,version_no,config,content_hash) SELECT ?,provider_id,preset_version_id,version_no+1,config,content_hash FROM catalog.provider_config_version WHERE id=?`, next, f.config).Error)
				require.NoError(t, database.Exec(`UPDATE catalog.provider SET current_config_version_id=? WHERE id=?`, next, f.provider).Error)
			case "credential_rotated":
				require.NoError(t, database.Exec(`UPDATE catalog.provider_credential SET status='draining' WHERE id=?`, f.credential).Error)
				require.NoError(t, database.Exec(`INSERT INTO catalog.provider_credential(id,provider_id,label,last4,ciphertext,key_id,status) VALUES(?,?,'fixture','test',decode('abcd','hex'),'test','active')`, uuid.New(), f.provider).Error)
			case "procurement_revoked":
				require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET is_delete=true WHERE id::text=(SELECT rule->>'procurement_price_version_id' FROM catalog.price_rule_version WHERE id=?)`, f.identity.PriceRuleVersionID).Error)
			case "price_revoked":
				require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET is_delete=true WHERE id=?`, f.identity.PriceRuleVersionID).Error)
			}
			if scenario == "settlement_interrupted" {
				require.NoError(t, database.Exec(`CREATE TRIGGER gateway_fail_settle BEFORE INSERT ON billing.ledger_entry FOR EACH ROW WHEN (NEW.entry_type='settle') EXECUTE FUNCTION pg_catalog.suppress_redundant_updates_trigger()`).Error)
				defer database.Exec(`DROP TRIGGER IF EXISTS gateway_fail_settle ON billing.ledger_entry`)
			}
			sends := 0
			client := &http.Client{Transport: adapterTransport(func(r *http.Request) (*http.Response, error) {
				sends++
				if strings.HasPrefix(scenario, "thinking_") || strings.HasPrefix(scenario, "max_tokens_") {
					var body map[string]json.RawMessage
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					if strings.HasPrefix(scenario, "thinking_") {
						require.JSONEq(t, `{"type":"disabled"}`, string(body["thinking"]))
					}
					if scenario == "max_tokens_bound" {
						require.Equal(t, "32768", string(body["max_tokens"]))
					}
				}
				require.Equal(t, "Bearer fixture-only-not-real", r.Header.Get("Authorization"))
				if scenario == "response_lost" || scenario == "response_lost_credential_changed" {
					return nil, errors.New("connection interrupted after POST")
				}
				if scenario == "identity_revoked_after_send" {
					require.NoError(t, database.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, scope.ActorID).Error)
				}
				return adapterResponse(200, `{"choices":[{"finish_reason":"tool_calls","message":{"content":null,"tool_calls":[{"id":"tool-id","type":"function","function":{"name":"read_context","arguments":"{}"}}]}}],"usage":{"prompt_tokens":7,"completion_tokens":3,"prompt_tokens_details":{"cached_tokens":2}}}`), nil
			})}
			gatewayDB := database
			if scenario == "runtime_role" {
				runtimeDSN := dsn
				if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
					parsed, err := url.Parse(dsn)
					require.NoError(t, err)
					q := parsed.Query()
					q.Set("options", "-c role=glowloom_app")
					parsed.RawQuery = q.Encode()
					runtimeDSN = parsed.String()
				} else {
					runtimeDSN += " options='-c role=glowloom_app'"
				}
				runtime, err := db.Open(t.Context(), runtimeDSN, noop.NewTracerProvider())
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, runtime.Close()) })
				gatewayDB = runtime.DB
				var role string
				require.NoError(t, gatewayDB.Raw(`SELECT current_user`).Row().Scan(&role))
				require.Equal(t, "glowloom_app", role)
			}
			operations := operationpg.NewScopedStore(gatewayDB, operationWorkerAccess, operationWorkerAccess, operationPlatformAccess).WithExecutionAvailability(func(tx *gorm.DB) op.ExecutionAvailability { return catalogpg.NewStore(tx) })
			if scenario == "availability_dependency_missing" {
				operations = operationpg.NewScopedStore(database, operationWorkerAccess, operationWorkerAccess, operationPlatformAccess)
			}
			bills := billingpg.NewScopedStore(gatewayDB, func(tx *gorm.DB) billing.ProjectAccess { return operationWorkerAccess(tx).(billing.ProjectAccess) })
			gateway := modelgateway.New(gatewayDB, operations, bills, fixtureUnsealer{}, client)
			result, err := gateway.Model(t.Context(), scope, id, input, maximum)
			if scenario == "thinking_enabled" || scenario == "thinking_auto" || scenario == "max_tokens_exceeded" || scenario == "unverified_mode" || scenario == "credential_rotated" || scenario == "credential_invalid" || scenario == "config_changed" || scenario == "price_revoked" || scenario == "procurement_revoked" || scenario == "availability_dependency_missing" || scenario == "budget_rejected" || scenario == "grant_revoked" || scenario == "wrong_release" || scenario == "params_tampered" || scenario == "model_tampered" || scenario == "tokens_tampered" || scenario == "skill_disabled_before_dispatch" {
				require.NoError(t, err)
				require.Equal(t, "failed", result.Status)
				require.Zero(t, sends)
				var n int64
				require.NoError(t, database.Raw(`SELECT count(*) FROM billing.reservation WHERE operation_id=?`, id).Scan(&n).Error)
				require.Zero(t, n)
				require.NoError(t, database.Raw(`SELECT count(*) FROM operation.operation WHERE id=?`, id).Scan(&n).Error)
				require.Zero(t, n)
				return
			}
			if scenario == "thinking_explicit" || scenario == "thinking_default" || scenario == "max_tokens_bound" {
				var frozen struct {
					Params    []byte
					InputHash string
				}
				require.NoError(t, database.Raw(`SELECT params,input_hash FROM operation.operation WHERE id=?`, id).Scan(&frozen).Error)
				var params map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(frozen.Params, &params))
				if strings.HasPrefix(scenario, "thinking_") {
					require.Equal(t, `"disabled"`, string(params["thinking"]))
				}
				if scenario == "max_tokens_bound" {
					require.Equal(t, "32768", string(params["max_tokens"]))
				}
				require.Equal(t, agent.Hash(input), frozen.InputHash)
			}
			if scenario == "complete" || scenario == "token_pricing" || scenario == "runtime_role" {
				var price catalog.PriceRuleVersion
				require.NoError(t, database.Raw(`SELECT id,model_profile_id AS model_id,version_no,unit,rule,currency,fx_rate_to_cny::text AS fx_rate_to_cny,effective_from FROM catalog.price_rule_version WHERE id=?`, f.identity.PriceRuleVersionID).Scan(&price).Error)
				ordinary, quoteErr := op.PrepareFreeQuote(op.CreateFreeQuoteInput{ProjectID: scope.ProjectID, RequestID: uuid.NewString(), FreeQuoteItemInput: op.FreeQuoteItemInput{ModelKey: model, Capability: "text.generate", Mode: "text_to_text", Prompt: string(input), Params: json.RawMessage(`{"max_tokens":100}`), OutputCount: 1}}, op.FreeQuoteCatalog{ReleaseID: f.release, ModelVersionID: f.identity.ModelProfileVersionID, ProviderModelID: "mock-v1", Region: "overseas", Modes: []string{"text_to_text"}, InputRoles: []string{"prompt"}, Limits: json.RawMessage(`{"max_outputs":1}`), ParamSchema: json.RawMessage(`[{"field":"max_tokens","label":"最大输出","type":"integer","component":"input","min":1,"max":1000}]`), Price: price}, nil, time.Now(), nil)
				require.NoError(t, quoteErr)
				var reserved struct {
					AmountMicros int64
					WholeCredits bool
					QuoteMicros  int64
				}
				require.NoError(t, database.Raw(`SELECT r.amount_micros,r.whole_credits,o.quote_micros FROM billing.reservation r JOIN operation.operation o ON o.id=r.operation_id WHERE r.operation_id=?`, id).Scan(&reserved).Error)
				require.True(t, reserved.WholeCredits)
				require.Equal(t, *ordinary.Operation.QuoteMicros, reserved.AmountMicros)
				require.Equal(t, *ordinary.Operation.QuoteMicros, reserved.QuoteMicros)
			}
			if scenario == "response_lost" || scenario == "response_lost_credential_changed" {
				if scenario == "response_lost_credential_changed" {
					require.NoError(t, database.Exec(`UPDATE catalog.provider_credential SET last_test_result='auth_failed' WHERE id=?`, f.credential).Error)
				}
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
			require.EqualValues(t, 100000, result.CostMicros)
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
			require.EqualValues(t, 100000, row.CostMicros)
			require.EqualValues(t, 100000, row.ReservedMicros)
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

// seedGatewayAvailability supplies synthetic committed evidence in an isolated
// PostgreSQL fixture. It never invokes an adapter or substitutes an allow stub.
func seedGatewayAvailability(t *testing.T, database *gorm.DB, f managedFixture, schema json.RawMessage) {
	t.Helper()
	require.NoError(t, database.Exec(`UPDATE catalog.provider SET current_config_version_id=? WHERE id=?`, f.config, f.provider).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET current_version_id=? WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, f.identity.ModelProfileVersionID, f.identity.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile_version SET limits='{"max_outputs":1}',param_schema=?::jsonb WHERE id=?`, string(schema), f.identity.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET is_delete=false,source_url='https://example.invalid/gateway-reviewed-price',reviewed_at=now()-interval '1 hour' WHERE id=? OR id::text=(SELECT rule->>'procurement_price_version_id' FROM catalog.price_rule_version WHERE id=?)`, f.identity.PriceRuleVersionID, f.identity.PriceRuleVersionID).Error)
	evidence := uuid.New()
	require.NoError(t, database.Exec(`INSERT INTO operation.operation(id,project_id,target_type,capability,mode,input_hash,origin,status,region,model_profile_version_id,price_rule_version_id) VALUES(?,?,'free','text.generate','text_to_text',?,'system','completed',(SELECT region FROM catalog.provider WHERE id=?),?,?)`, evidence, f.identity.ProjectID, uuid.NewString(), f.provider, f.identity.ModelProfileVersionID, f.identity.PriceRuleVersionID).Error)
	require.NoError(t, database.Exec(`INSERT INTO operation.provider_binding(operation_id,release_id) VALUES(?,?)`, evidence, f.release).Error)
	require.NoError(t, database.Exec(`INSERT INTO operation.provider_credential_binding(operation_id,credential_id) VALUES(?,?)`, evidence, f.credential).Error)
	require.NoError(t, database.Exec(`INSERT INTO catalog.model_validation(id,release_id,credential_id,adapter_version,mode,input_digest,operation_id,status,evidence,completed_at) VALUES(?,?,?,'ai-provider-v1','text_to_text',?,?,'succeeded','{"kind":"real_provider"}',now())`, uuid.New(), f.release, f.credential, uuid.NewString(), evidence).Error)
	require.NoError(t, database.Transaction(func(tx *gorm.DB) error {
		available, err := catalogpg.NewStore(tx).AvailableForNewExecution(t.Context(), f.identity.ModelProfileVersionID, f.release, "text_to_text")
		require.NoError(t, err)
		require.True(t, available, "synthetic gateway fixture must satisfy the real catalog projection")
		return nil
	}))
}
