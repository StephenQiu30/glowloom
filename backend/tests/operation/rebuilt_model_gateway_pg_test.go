package operation_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/creation/evidence"
	evidencepg "github.com/StephenQiu30/glowloom-server/backend/creation/evidence/adapter/postgres"
	workspacepg "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/postgres"
	pg "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/postgres"
	aiapp "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	ai "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	agentpg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	agent "github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	agentwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/agent"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

type rebuiltProvider func(context.Context, aiapp.Dispatch, func(json.RawMessage) error) (ai.Receipt, error)

func (f rebuiltProvider) Send(ctx context.Context, d aiapp.Dispatch, emit func(json.RawMessage) error) (ai.Receipt, error) {
	return f(ctx, d, emit)
}

// This is the CI legacy-DSN transition suite. Product invariants now exercise the
// production Agent entrypoint and ai ledger; old reservation/runtime assertions
// are retired explicitly in final-fix-report.md, never restored as a funds path.
func TestModelGatewayPersistsDialogueUsageAndAtomicSettlement(t *testing.T) {
	for _, scenario := range []string{"complete", "runtime_role", "memory_changed", "evidence_changed", "evidence_current", "pool_backup", "pool_pinned", "response_lost", "response_lost_credential_changed", "settlement_interrupted", "budget_rejected", "grant_revoked", "wrong_release", "params_tampered", "model_tampered", "tokens_tampered", "identity_revoked_before_send", "identity_revoked_after_send", "skill_disabled_before_dispatch", "skill_enabled_before_dispatch", "thinking_explicit", "max_tokens_bound", "max_tokens_exceeded", "credential_invalid", "price_frozen"} {
		t.Run(scenario, func(t *testing.T) {
			dsn := os.Getenv("GLOWLOOM_TEST_MODEL_GATEWAY_DB_DSN")
			if dsn == "" {
				t.Skip("requires isolated model gateway transition PostgreSQL")
			}
			conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, conn.Close()) })
			database := conn.DB
			a := identity.Principal{ID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New()}
			p := uuid.New()
			require.NoError(t, database.Exec("INSERT INTO workspace.organization(id,name) VALUES(?,'AI transition')", a.OrgID).Error)
			require.NoError(t, database.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password,email_key,email_verified_at) VALUES(?,?,?,'AI transition','producer','synthetic',false,?,clock_timestamp())`, a.ID, a.OrgID, a.ID.String(), a.ID.String()+"@transition.invalid").Error)
			require.NoError(t, database.Exec("INSERT INTO workspace.creation_scope(id,kind,owner_user_id,physical_scope_id) VALUES(?,'personal',?,?)", a.ScopeID, a.ID, a.OrgID).Error)
			require.NoError(t, database.Exec("INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'AI transition','16:9','realistic')", p, a.OrgID, a.ID, a.ScopeID).Error)
			publish := pg.New(database, a, func(context.Context, *gorm.DB, identity.Principal, uuid.UUID, uuid.UUID, bool) error { return nil })
			price := ai.Price{Input: 1000000, CacheRead: 100000, CacheWrite: 2000000, Output: 2000000, Currency: "CNY", FX: "1", Multiplier: "1", Discount: "1"}
			m := ai.Model{ID: uuid.New(), Name: "transition-" + uuid.NewString(), Protocol: "chat", Price: price, MaxInput: 8192, MaxOutput: 32768, PurchaseCap: 100000, Tools: true}
			require.NoError(t, publish.PublishModel(t.Context(), m))
			require.NoError(t, publish.Grant(t.Context(), a.ScopeID, "fund", 100))
			routes := []ai.Route{}
			for i := range 2 {
				route := ai.Route{ID: uuid.New(), ModelID: m.ID, Supplier: uuid.New(), Credential: uuid.New(), Group: uuid.NewString(), Protocol: "chat", BaseURL: "https://8.8.8.8/v1", UpstreamModel: "upstream", Purchase: price, Priority: i}
				require.NoError(t, publish.PublishRoute(t.Context(), route, []byte("fixture"), "fixture", 10, 10000))
				routes = append(routes, route)
			}
			root, segment, id, session := uuid.New(), uuid.New(), uuid.New(), uuid.New()
			evidenceReceipt := uuid.New()
			scope := agent.Scope{ActorID: a.ID, OrgID: a.OrgID, ScopeID: a.ScopeID, ProjectID: p}
			scopeRaw, _ := json.Marshal(scope)
			params := map[string]any{}
			tokens := 100
			if scenario == "max_tokens_bound" {
				tokens = 32768
			}
			if scenario == "max_tokens_exceeded" {
				tokens = 32769
			}
			if scenario == "thinking_explicit" {
				params["thinking"] = "disabled"
			}
			selection, _ := json.Marshal(map[string]any{"model_key": m.Name, "release_id": m.ID, "max_tokens": tokens, "params": params})
			input := map[string]any{"model_key": m.Name, "release_id": m.ID, "max_tokens": tokens, "params": params, "messages": []any{map[string]any{"role": "user", "content": "read the scene"}}, "tools": []any{map[string]any{"name": "read_context", "parameters": map[string]any{"type": "object"}}}}
			switch scenario {
			case "evidence_changed", "evidence_current":
				input["project_evidence_receipts"] = []uuid.UUID{evidenceReceipt}
			case "runtime_role", "memory_changed":
				input["project_memory_version"] = workspacepg.ProjectMemoryVersion(nil)
			case "wrong_release":
				input["release_id"] = uuid.New()
			case "model_tampered":
				input["model_key"] = "other"
			case "tokens_tampered":
				input["max_tokens"] = 101
			case "params_tampered":
				input["params"] = map[string]any{"temperature": 0.2}
			}
			raw, _ := json.Marshal(input)
			maximum := int64(1000000)
			if scenario == "budget_rejected" {
				maximum = 1
			}
			require.NoError(t, database.Exec("INSERT INTO agent_business.run_authorization(root_run_id,session_id,submission_id,request_id,content_hash,scope,budget_micros,owner_epoch,model_selection) VALUES(?,?,?,?,?,?::jsonb,2000000,1,?::jsonb)", root, session, segment, uuid.New(), agent.Hash(raw), string(scopeRaw), string(selection)).Error)
			require.NoError(t, database.Exec("INSERT INTO agent_business.execution_grant(grant_id,root_run_id,submission_id,owner_epoch,authorization_revision,expires_at) VALUES(?,?,?,1,1,clock_timestamp()+interval '1 hour')", uuid.New(), root, segment).Error)
			require.NoError(t, database.Exec("INSERT INTO agent_business.call_receipt(receipt_id,root_run_id,submission_id,task_id,call_id,checkpoint_id,attempt,contract_version,input_hash,kind,name,max_micros,status) VALUES(?,?,?,'task','call','checkpoint',1,'v1',?,'model','model',?,'in_flight')", id, root, segment, agent.Hash(raw), maximum).Error)
			if strings.HasPrefix(scenario, "evidence_") {
				item, err := evidencepg.Read(database, p, evidence.Reference{Kind: "brief", ID: p})
				require.NoError(t, err)
				require.Len(t, item.Items, 1)
				result, err := json.Marshal(item)
				require.NoError(t, err)
				require.NoError(t, database.Exec(`INSERT INTO agent_business.call_receipt(
					receipt_id,root_run_id,submission_id,task_id,call_id,checkpoint_id,attempt,contract_version,input_hash,kind,name,max_micros,status,result)
					VALUES(?,?,?,'search','evidence','checkpoint',1,'v1',?,'tool','read_project_evidence',0,'completed',?::jsonb)`,
					evidenceReceipt, root, segment, agent.Hash(raw), string(result)).Error)
				if scenario == "evidence_changed" {
					require.NoError(t, database.Exec("UPDATE workspace.project SET description='new source',revision=revision+1 WHERE id=?", p).Error)
				}
			}
			if scenario == "memory_changed" {
				require.NoError(t, database.Exec(`INSERT INTO workspace.project_memory_revision
					(id,project_id,key,revision,statement,withdrawn,confirmed_by,confirmed_at,request_id)
					VALUES (?,?,'audio',1,'no narration',false,?,clock_timestamp(),?)`, uuid.New(), p, a.ID, uuid.New()).Error)
			}
			if scenario == "grant_revoked" {
				require.NoError(t, database.Exec("UPDATE agent_business.run_authorization SET barrier=true WHERE root_run_id=?", root).Error)
			}
			if scenario == "identity_revoked_before_send" {
				require.NoError(t, database.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, a.ID).Error)
			}
			if strings.HasPrefix(scenario, "skill_") {
				pin := agent.SkillPin{SkillID: uuid.New(), Version: "1", ResourceHash: agent.Hash([]byte("fixture"))}
				pins, _ := json.Marshal([]agent.SkillPin{pin})
				require.NoError(t, database.Exec("INSERT INTO agent_business.skill_version(skill_id,version,resource_hash,owner_actor_id,org_id,scope_id,name,description,body,enabled) VALUES(?,?,?,?,?,?,'fixture','fixture','fixture',?)", pin.SkillID, pin.Version, pin.ResourceHash, a.ID, a.OrgID, a.ScopeID, scenario == "skill_enabled_before_dispatch").Error)
				require.NoError(t, database.Exec("UPDATE agent_business.run_authorization SET skill_pins=?::jsonb WHERE root_run_id=?", string(pins), root).Error)
			}
			if scenario == "pool_backup" {
				require.NoError(t, database.Exec("UPDATE ai.credential SET state='disabled' WHERE id=?", routes[0].Credential).Error)
			}
			if scenario == "credential_invalid" {
				require.NoError(t, database.Exec("UPDATE ai.credential SET state='disabled' WHERE id IN ?", []uuid.UUID{routes[0].Credential, routes[1].Credential}).Error)
			}
			if scenario == "runtime_role" {
				database = database.Session(&gorm.Session{NewDB: true})
				sqlDB, e := database.DB()
				require.NoError(t, e)
				sqlDB.SetMaxOpenConns(1)
				require.NoError(t, database.Exec("SET ROLE glowloom_app").Error)
				t.Cleanup(func() { require.NoError(t, database.Exec("RESET ROLE").Error) })
			}
			sends := 0
			fault := false
			callback := "transition-" + id.String()
			require.NoError(t, database.Callback().Raw().Before("gorm:raw").Register(callback, func(tx *gorm.DB) {
				if fault && strings.HasPrefix(tx.Statement.SQL.String(), "UPDATE ai.account") {
					_ = tx.AddError(errors.New("settlement interruption"))
				}
			}))
			t.Cleanup(func() { require.NoError(t, database.Callback().Raw().Remove(callback)) })
			runtime := &aigateway.Runtime{DB: database, Provider: rebuiltProvider(func(_ context.Context, d aiapp.Dispatch, _ func(json.RawMessage) error) (ai.Receipt, error) {
				sends++
				require.Contains(t, string(d.Call.Raw), "read the scene")
				require.NotContains(t, string(d.Call.Raw), "project_memory_version")
				require.NotContains(t, string(d.Call.Raw), "project_evidence_receipts")
				if scenario == "pool_backup" {
					require.Equal(t, routes[1].ID, d.Route.ID)
				}
				if scenario == "pool_pinned" {
					require.Equal(t, routes[0].ID, d.Route.ID)
				}
				if scenario == "identity_revoked_after_send" {
					require.NoError(t, database.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, a.ID).Error)
				}
				if strings.HasPrefix(scenario, "response_lost") {
					return ai.Receipt{}, errors.New("response lost after send")
				}
				if scenario == "settlement_interrupted" {
					fault = true
				}
				if scenario == "price_frozen" {
					next := m
					next.ID = uuid.New()
					next.Price.Input = 9000000
					require.NoError(t, publish.PublishModel(t.Context(), next))
				}
				u := ai.Usage{Input: 10, Output: 2, CacheRead: 2}
				cost, e := d.Route.Purchase.Cost(u)
				return ai.Receipt{Body: json.RawMessage(`{"choices":[{"message":{"content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"read_context","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`), Usage: u, PurchaseMicros: cost}, e
			})}
			result, err := runtime.AgentModel(t.Context(), scope, id, raw, maximum)
			require.NoError(t, err)
			rejected := []string{"memory_changed", "evidence_changed", "budget_rejected", "grant_revoked", "wrong_release", "params_tampered", "model_tampered", "tokens_tampered", "identity_revoked_before_send", "skill_disabled_before_dispatch", "thinking_explicit", "max_tokens_exceeded", "credential_invalid"}
			if slices.Contains(rejected, scenario) {
				require.Equal(t, agent.CallStatusFailed, result.Status)
				if scenario == "memory_changed" {
					require.JSONEq(t, `{"code":"project_memory_changed"}`, string(result.Result))
				}
				if scenario == "evidence_changed" {
					require.JSONEq(t, `{"code":"project_evidence_changed"}`, string(result.Result))
				}
				require.Zero(t, sends)
				var n int64
				require.NoError(t, database.Raw("SELECT count(*) FROM ai.request WHERE id=?", id).Scan(&n).Error)
				require.Zero(t, n)
				return
			}
			require.Equal(t, 1, sends)
			if strings.HasPrefix(scenario, "response_lost") {
				require.Equal(t, agent.CallStatusUnknown, result.Status)
				if scenario == "response_lost_credential_changed" {
					require.NoError(t, database.Exec("UPDATE ai.credential SET state='disabled' WHERE id=?", routes[0].Credential).Error)
				}
				for range 2 {
					r, e := runtime.AgentRecover(t.Context(), id)
					require.NoError(t, e)
					require.Equal(t, agent.CallStatusUnknown, r.Status)
				}
				var held int64
				require.NoError(t, database.Raw("SELECT held FROM ai.account WHERE scope_id=?", a.ScopeID).Scan(&held).Error)
				require.Greater(t, held, int64(0))
				require.Equal(t, 1, sends)
				return
			}
			if scenario == "settlement_interrupted" {
				require.Equal(t, agent.CallStatusUnknown, result.Status)
				fault = false
				result, err = runtime.AgentRecover(t.Context(), id)
				require.NoError(t, err)
			}
			require.Equal(t, agent.CallStatusCompleted, result.Status)
			require.EqualValues(t, 100000, result.CostMicros)
			require.Contains(t, string(result.Result), `"cache_read_tokens":2`)
			require.Contains(t, string(result.Result), `"id":"call-1"`)
			for range 2 {
				require.NoError(t, database.Transaction(func(tx *gorm.DB) error { _, e := agentpg.CompleteCall(tx, id, result); return e }))
				replay, e := runtime.AgentRecover(t.Context(), id)
				require.NoError(t, e)
				require.Equal(t, result, replay)
			}
			var facts struct{ Held, Charged, Settles, Reserved, OldFunds int64 }
			require.NoError(t, database.Raw("SELECT a.held,h.charged,(SELECT count(*) FROM ai.money_event WHERE request_id=h.request_id AND kind='settle') AS settles,(SELECT reserved_micros FROM agent_business.run_authorization WHERE root_run_id=?) AS reserved,(SELECT count(*) FROM billing.reservation WHERE operation_id=?) AS old_funds FROM ai.account a JOIN ai.hold h ON h.request_id=? WHERE a.scope_id=?", root, id, id, a.ScopeID).Scan(&facts).Error)
			require.Zero(t, facts.Held)
			require.EqualValues(t, 1, facts.Charged)
			require.EqualValues(t, 1, facts.Settles)
			require.EqualValues(t, 100000, facts.Reserved)
			require.Zero(t, facts.OldFunds)
			if scenario != "identity_revoked_after_send" {
				snapshot, e := agentwire.NewBoundary(database, nil).Store.CostSnapshot(t.Context(), root)
				require.NoError(t, e)
				require.EqualValues(t, 100000, snapshot.SettledCharge)
				require.Zero(t, snapshot.PendingCommitment)
				expectedEvidence := 1
				if scenario == "evidence_current" {
					expectedEvidence++
				}
				require.Len(t, snapshot.Evidence, expectedEvidence)
				var settled bool
				for _, item := range snapshot.Evidence {
					if item.OperationID != nil && *item.OperationID == id && item.SettlementID != nil {
						settled = true
					}
				}
				require.True(t, settled, "model settlement missing")
			}
		})
	}
}
