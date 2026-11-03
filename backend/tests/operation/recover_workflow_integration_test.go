package operation_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	temporalworkflow "go.temporal.io/sdk/workflow"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	inboxpg "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/adapter/postgres"
	inbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
	operationevent "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/event"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/managed"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

type billingWorkflowSignal struct {
	env *testsuite.TestWorkflowEnvironment
}

func (s billingWorkflowSignal) SignalWorkflow(_ context.Context, _, _, signal string, arg interface{}) error {
	s.env.SignalWorkflow(signal, arg)
	return nil
}

// Exercises the real PostgreSQL store, HTTP control and event consumer, Temporal
// workflow, managed activity, frozen credentials and actual protocol driver.
// Transport is synthetic; this is not a live supplier acceptance test.
func TestRecoverNeverCreatesGenerationThroughOriginalWorkflow(t *testing.T) {
	db := operationStoreDB(t)
	f := seedManagedFixture(t, db)
	id, pid := f.identity.OperationID, f.identity.ProjectID
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile SET capability='video.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, f.identity.ModelProfileVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text2video'],queue='provider',supports_query=true,supports_cancel=false,moderation='provider',expected_max_ms=1 WHERE id=?`, f.identity.ModelProfileVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE operation.operation SET status='confirmed',capability='video.generate',mode='text2video',params='{}' WHERE id=?`, id).Error)
	var actor identityapp.Principal
	require.NoError(t, db.Raw(`SELECT u.id,u.org_id,u.role FROM identity."user" u JOIN operation.operation o ON o.confirmed_by=u.id WHERE o.id=?`, id).Scan(&actor).Error)
	actor = identityfixture.Session(t, db, actor)
	router := publicOperationRouter(actor, db)
	posts, gets := 0, 0
	client := providerhttp.NewClient(&http.Client{Transport: adapterTransport(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "Bearer fixture-only-not-real", r.Header.Get("Authorization"))
		require.Contains(t, r.URL.Host, "openrouter")
		if r.Method == "POST" {
			posts++
			return adapterResponse(200, `{"id":"original-paid-task"}`), nil
		}
		gets++
		require.Contains(t, r.URL.Path, "original-paid-task")
		return adapterResponse(200, `{"id":"original-paid-task","status":"pending"}`), nil
	})})
	results := application.NewManagedResults(&stageTestObjects{entries: map[string]stageTestObject{}}, f.store)
	provider := managed.NewActivities(f.store, fixtureUnsealer{}, client, results, nil, nil)
	flow := operationflow.NewActivities(f.store, app.NewOperationFinalizer(db, f.store, billingpg.NewStore(db)))
	env := (&testsuite.WorkflowTestSuite{}).NewTestWorkflowEnvironment()
	register := func(name string, fn any) { env.RegisterActivityWithOptions(fn, activity.RegisterOptions{Name: name}) }
	register("flow.LoadOperation", flow.LoadOperation)
	register("flow.CheckConsent", flow.CheckConsent)
	register("flow.Transition", flow.Transition)
	register("flow.BeginProviderCall", flow.BeginProviderCall)
	register("flow.CompleteProviderCall", flow.CompleteProviderCall)
	register("flow.CheckManualNotExecuted", flow.CheckManualNotExecuted)
	register("flow.LoadProviderCost", flow.LoadProviderCost)
	register("flow.SettleOperation", flow.SettleOperation)
	register("provider.managed", func(ctx context.Context, step application.ManagedStep) (application.ManagedProgress, error) {
		result, err := provider.Step(ctx, step)
		// The provider accepted and durable evidence was saved, but the activity
		// response was lost. Workflow recovery must read the original receipt.
		if step.Action == "submit" && err == nil {
			return application.ManagedProgress{}, errors.New("fixture activity reply interrupted")
		}
		return result, err
	})
	var before dispatchBudgetSnapshot
	env.RegisterDelayedCallback(func() {
		var err error
		before, err = readDispatchBudget(t.Context(), db, id)
		require.NoError(t, err)
		require.Equal(t, 1, posts)
		refresh := publicOperationRequest(t, router, "GET", fmt.Sprintf("/api/operations/%s?project_id=%s", id, pid), "", "")
		require.Equal(t, 200, refresh.Code)
		require.Contains(t, refresh.Body.String(), `"regenerate":"blocked"`)
		require.NoError(t, db.Exec(`UPDATE catalog.provider_credential SET status='draining' WHERE id=?`, f.credential).Error)
		replacement := uuid.New()
		require.NoError(t, db.Exec(`INSERT INTO catalog.provider_credential(id,provider_id,label,last4,ciphertext,key_id,status) VALUES(?,?,'replacement','fake',decode('aaaa','hex'),'replacement','active')`, replacement, f.provider).Error)
		// A current release/config change cannot switch the original binding.
		require.NoError(t, db.Exec(`UPDATE catalog.model_profile SET active_release_id=NULL WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, f.identity.ModelProfileVersionID).Error)
		key := uuid.NewString()
		body := fmt.Sprintf(`{"project_id":%q}`, pid)
		path := "/api/operations/" + id.String() + "/recover"
		request := publicOperationRequest(t, router, "POST", path, body, key)
		require.Equal(t, 202, request.Code, request.Body.String())
		replay := publicOperationRequest(t, router, "POST", path, body, key)
		require.JSONEq(t, request.Body.String(), replay.Body.String())
		var event struct {
			Topic, PartitionKey string
			Payload             []byte
		}
		require.NoError(t, db.Raw(`SELECT topic,partition_key,payload FROM infra.outbox WHERE topic=? AND payload->'data'->>'request_id'=?`, operationevent.WorkflowControlTopic, key).Scan(&event).Error)
		handler := operationevent.NewWorkflowControlHandler(inboxpg.NewStore(db), f.store, operationflow.NewControl(billingWorkflowSignal{env}))
		require.NoError(t, handler.Handle(t.Context(), inbox.Record{Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Payload}))
	}, 3*time.Minute)
	env.RegisterDelayedCallback(func() {
		require.Greater(t, gets, 1)
		require.Equal(t, 1, posts)
		saved, err := f.store.LoadManagedCredential(t.Context(), id)
		require.NoError(t, err)
		require.Equal(t, f.credential, saved.CredentialID)
		require.Equal(t, f.config, saved.ConfigVersionID)
		require.NoError(t, db.Exec(`UPDATE catalog.provider_credential SET status='disabled' WHERE id=?`, f.credential).Error)
		previous := gets
		_, err = provider.Step(t.Context(), application.ManagedStep{OperationID: id, Action: "query"})
		require.ErrorIs(t, err, application.ErrWorkflowModelUnavailable)
		require.Equal(t, previous, gets)
		env.CancelWorkflow()
	}, 4*time.Minute)
	env.SetTestTimeout(20 * time.Second)
	env.ExecuteWorkflow(operationflow.OperationWorkflow, operationflow.OperationInput{OperationID: id.String()})
	require.Error(t, env.GetWorkflowError())
	require.Equal(t, 1, posts)
	after, err := readDispatchBudget(t.Context(), db, id)
	require.NoError(t, err)
	require.Equal(t, before, after)
	var submits int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM operation.provider_call WHERE operation_id=? AND action='submit'`, id).Scan(&submits).Error)
	require.Equal(t, int64(1), submits)
}
func TestSentProviderErrorRetainsReservation(t *testing.T) {
	for _, tc := range []struct {
		status     int
		body, code string
	}{{0, "", "local_not_sent"}, {429, "{}", "legacy_rejection_history"}, {429, "{}", "provider_request_limited"}, {402, "{}", "provider_payment_required"}, {429, `{"error":{"code":"credit_balance_exhausted"}}`, "provider_insufficient_balance"}} {
		t.Run(tc.code, func(t *testing.T) {
			db := operationStoreDB(t)
			f := seedManagedFixture(t, db)
			id, pid := f.identity.OperationID, f.identity.ProjectID
			require.NoError(t, db.Exec(`UPDATE catalog.model_profile SET capability='video.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, f.identity.ModelProfileVersionID).Error)
			require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text2video'],queue='provider',supports_query=true,supports_cancel=false,moderation='provider',expected_max_ms=1 WHERE id=?`, f.identity.ModelProfileVersionID).Error)
			require.NoError(t, db.Exec(`UPDATE operation.operation SET status='confirmed',capability='video.generate',mode='text2video',params='{}' WHERE id=?`, id).Error)
			if tc.status == 0 {
				require.NoError(t, db.Exec(`UPDATE operation.operation SET output_count=2 WHERE id=?`, id).Error)
			}
			var actor identityapp.Principal
			require.NoError(t, db.Raw(`SELECT u.id,u.org_id,u.role FROM identity."user" u JOIN operation.operation o ON o.confirmed_by=u.id WHERE o.id=?`, id).Scan(&actor).Error)
			actor = identityfixture.Session(t, db, actor)
			router := publicOperationRouter(actor, db)
			source := domain.CanvasSource{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1}
			if tc.code == "provider_request_limited" {
				encoded, err := json.Marshal(source)
				require.NoError(t, err)
				require.NoError(t, db.Exec(`INSERT INTO canvas.document(id,project_id,name,revision) VALUES(?,?,'sent rejection fixture',1)`, source.CanvasID, pid).Error)
				require.NoError(t, db.Exec(`INSERT INTO canvas.node(id,document_id,node_type,config,x,y) VALUES(?,?,'note','{}',0,0)`, source.NodeID, source.CanvasID).Error)
				require.NoError(t, db.Exec(`UPDATE operation.operation SET source_context=?::jsonb WHERE id=?`, string(encoded), id).Error)
			}
			posts, gets := 0, 0
			client := providerhttp.NewClient(&http.Client{Transport: adapterTransport(func(r *http.Request) (*http.Response, error) {
				require.Equal(t, "Bearer fixture-only-not-real", r.Header.Get("Authorization"))
				require.Contains(t, r.URL.Host, "openrouter")
				if r.Method == "POST" {
					posts++
					return adapterResponse(tc.status, tc.body), nil
				}
				gets++
				require.Contains(t, r.URL.Path, "original-paid-task")
				return adapterResponse(200, `{"id":"original-paid-task","status":"pending"}`), nil
			})})
			results := application.NewManagedResults(&stageTestObjects{entries: map[string]stageTestObject{}}, f.store)
			provider := managed.NewActivities(f.store, fixtureUnsealer{}, client, results, nil, nil)
			flow := operationflow.NewActivities(f.store, app.NewOperationFinalizer(db, f.store, billingpg.NewStore(db)))
			env := (&testsuite.WorkflowTestSuite{}).NewTestWorkflowEnvironment()
			register := func(name string, fn any) { env.RegisterActivityWithOptions(fn, activity.RegisterOptions{Name: name}) }
			register("flow.LoadOperation", flow.LoadOperation)
			register("flow.CheckConsent", flow.CheckConsent)
			register("flow.Transition", flow.Transition)
			register("flow.BeginProviderCall", flow.BeginProviderCall)
			register("flow.CompleteProviderCall", flow.CompleteProviderCall)
			register("flow.CheckManualNotExecuted", flow.CheckManualNotExecuted)
			register("flow.LoadProviderCost", flow.LoadProviderCost)
			settles := 0
			register("flow.SettleOperation", func(ctx context.Context, in operationflow.SettlementInput) error {
				settles++
				return flow.SettleOperation(ctx, in)
			})
			if tc.code == "legacy_rejection_history" {
				env.OnGetVersion("managed-sent-rejection-v1", temporalworkflow.DefaultVersion, 1).Return(temporalworkflow.DefaultVersion).Once()
			}
			register("provider.managed", func(ctx context.Context, step application.ManagedStep) (application.ManagedProgress, error) {
				progress, err := provider.Step(ctx, step)
				if tc.code == "legacy_rejection_history" && step.Action == "submit" && progress.State == "unknown" {
					progress.State = "rejected"
				}
				return progress, err
			})
			env.RegisterDelayedCallback(func() { env.CancelWorkflow() }, 3*time.Minute)
			env.SetTestTimeout(10 * time.Second)
			env.ExecuteWorkflow(operationflow.OperationWorkflow, operationflow.OperationInput{OperationID: id.String()})
			res := publicOperationRequest(t, router, "GET", fmt.Sprintf("/api/operations/%s?project_id=%s", id, pid), "", "")
			var row struct{ Charge, Status string }
			require.NoError(t, db.Raw(`SELECT o.status,r.status AS charge FROM operation.operation o JOIN billing.reservation r ON r.operation_id=o.id WHERE o.id=?`, id).Scan(&row).Error)
			t.Logf("POST=%d GET=%d actual operation=%s reservation=%s", posts, gets, row.Status, row.Charge)
			if tc.status == 0 {
				require.Zero(t, posts)
				require.Zero(t, gets)
				require.Equal(t, "released", row.Charge)
				require.Equal(t, "failed", row.Status)
				require.Contains(t, res.Body.String(), `"regenerate":"confirm"`)
				return
			}
			require.Equal(t, "held", row.Charge)
			require.Contains(t, res.Body.String(), `"regenerate":"blocked"`)
			require.Equal(t, 1, posts)
			require.Zero(t, gets)
			require.Equal(t, "manual", row.Status)
			if tc.code == "legacy_rejection_history" {
				require.Equal(t, 1, settles)
				env.AssertExpectations(t)
			} else {
				require.Contains(t, res.Body.String(), tc.code)
				require.Zero(t, settles)
			}
			var count int64
			require.NoError(t, db.Raw(`SELECT count(*) FROM operation.provider_call WHERE operation_id=? AND action='submit' AND outcome='unknown' AND cost_micros IS NULL`, id).Scan(&count).Error)
			require.EqualValues(t, 1, count)
			if tc.code != "provider_request_limited" {
				return
			}
			before, err := readDispatchBudget(t.Context(), db, id)
			require.NoError(t, err)
			late := application.CompleteProviderCallInput{OperationID: id, Action: "submit", Attempt: 1, Outcome: "error", State: "not_submitted"}
			require.ErrorIs(t, f.store.CompleteProviderCall(t.Context(), late), application.ErrProviderCallConflict)
			err = app.NewOperationFinalizer(db, f.store, billingpg.NewStore(db)).FinalizeOperation(t.Context(), operationflow.SettlementInput{OperationID: id.String(), From: domain.StatusManual, To: domain.StatusFailed, FailureCode: "provider_not_submitted"})
			require.ErrorIs(t, err, application.ErrManualResolutionUnverified)
			after, err := readDispatchBudget(t.Context(), db, id)
			require.NoError(t, err)
			require.Equal(t, before, after)
			res = publicOperationRequest(t, router, "GET", fmt.Sprintf("/api/operations/%s?project_id=%s", id, pid), "", "")
			require.Equal(t, 200, res.Code)
			require.Contains(t, res.Body.String(), tc.code)
			require.Contains(t, res.Body.String(), `"regenerate":"blocked"`)
			require.NoError(t, db.Raw(`SELECT count(*) FROM operation.provider_call WHERE operation_id=? AND action='submit' AND outcome='unknown' AND response_summary->>'state'='unknown' AND cost_micros IS NULL`, id).Scan(&count).Error)
			require.EqualValues(t, 1, count)
			batch, items := seedProjectBatch(t, db, actor, pid, 1)
			single := items[0]
			single.Operation.BatchID = nil
			single.Operation.ID = uuid.New()
			single.Inputs = append([]domain.OperationInput(nil), single.Inputs...)
			single.Inputs[0].ID = uuid.New()
			single.Inputs[0].OperationID = single.Operation.ID
			require.NoError(t, f.store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{single}))
			encoded, err := json.Marshal(source)
			require.NoError(t, err)
			require.NoError(t, db.Exec(`UPDATE operation.operation SET source_context=?::jsonb WHERE id IN ?`, string(encoded), []uuid.UUID{single.Operation.ID, items[0].Operation.ID}).Error)
			assertPaidAdmissionBlocked(t, db, actor, pid, batch, single, source)
			require.Equal(t, 1, posts)

		})
	}
}
