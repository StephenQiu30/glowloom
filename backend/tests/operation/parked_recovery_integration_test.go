package operation_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/managed"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestExplicitRecoverReopensOnlyOriginalParkedReadWorkOnce(t *testing.T) {
	for _, scenario := range []string{"query", "unknown", "no_dispatch", "wrong_task", "wrong_identity", "active_lease", "completed_work", "revoked"} {
		t.Run(scenario, func(t *testing.T) {
			db := operationStoreDB(t)
			f := seedUnverifiedManagedFixture(t, db)
			id, pid := f.identity.OperationID, f.identity.ProjectID
			require.NoError(t, db.Exec(`UPDATE catalog.model_profile SET capability='video.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, f.identity.ModelProfileVersionID).Error)
			require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text2video'],queue='provider',supports_query=true,supports_cancel=false,moderation='provider',expected_max_ms=60000 WHERE id=?`, f.identity.ModelProfileVersionID).Error)
			require.NoError(t, db.Exec(`UPDATE operation.operation SET status='confirmed',capability='video.generate',mode='text2video',params='{}' WHERE id=?`, id).Error)
			seedDispatchModeEvidence(t, db, f)
			var actor identityapp.Principal
			require.NoError(t, db.Raw(`SELECT u.id,u.org_id,u.role FROM identity."user" u JOIN operation.operation o ON o.confirmed_by=u.id WHERE o.id=?`, id).Scan(&actor).Error)
			actor = identityfixture.Session(t, db, actor)
			posts, gets := 0, 0
			client := providerhttp.NewClient(&http.Client{Transport: adapterTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method == http.MethodPost {
					posts++
					return adapterResponse(200, `{"id":"original-paid-task"}`), nil
				}
				gets++
				require.Contains(t, r.URL.Path, "original-paid-task")
				return adapterResponse(200, `{"id":"original-paid-task","status":"pending"}`), nil
			})})
			results := application.NewManagedResults(&stageTestObjects{entries: map[string]stageTestObject{}}, f.store)
			h := map[string]taskruntime.Handler{}
			operationflow.RegisterFlow(h, f.store, app.NewOperationFinalizer(db, f.store, billingpg.NewStore(db)), nil)
			operationflow.RegisterProvider(h, f.store, managed.NewActivities(f.store, fixtureUnsealer{}, client, results, nil))
			j := taskruntime.Job{BusinessID: id.String()}
			for _, kind := range []string{"operation.run", "operation.provider", "operation.run"} {
				_, err := h[kind](t.Context(), j)
				require.NoError(t, err)
			}
			require.Equal(t, 1, posts)
			require.NoError(t, f.store.ScheduleOperationStep(t.Context(), "operation.provider", id))
			require.NoError(t, db.Exec(`UPDATE infra.task_work SET failed_at=now(),failures=6,epoch=7,attempt=14,owner=NULL,lease_until=NULL WHERE kind='operation.provider' AND business_id=?`, id.String()).Error)
			switch scenario {
			case "unknown":
				require.NoError(t, db.Exec(`UPDATE operation.operation SET status='manual' WHERE id=?`, id).Error)
				require.NoError(t, db.Exec(`UPDATE operation.provider_call SET provider_task_id=NULL,response_summary='{"state":"unknown"}' WHERE operation_id=? AND action='submit'`, id).Error)
			case "no_dispatch":
				require.NoError(t, db.Exec(`UPDATE operation.provider_call SET dispatch_started_at=NULL WHERE operation_id=? AND action='submit'`, id).Error)
			case "wrong_task":
				require.NoError(t, db.Exec(`UPDATE operation.operation_event SET detail=jsonb_set(detail,'{provider_task_id}','"other-task"') WHERE operation_id=? AND detail ? 'provider_task_id'`, id).Error)
			case "wrong_identity":
				require.NoError(t, db.Exec(`UPDATE operation.provider_call SET request_key='other-key' WHERE operation_id=? AND action='submit'`, id).Error)
			case "completed_work":
				require.NoError(t, db.Exec(`UPDATE infra.task_work SET completed_at=now() WHERE kind='operation.provider' AND business_id=?`, id.String()).Error)
			case "active_lease":
				require.NoError(t, db.Exec(`UPDATE infra.task_work SET owner='old-owner',lease_until=now()+interval '1 hour' WHERE kind='operation.provider' AND business_id=?`, id.String()).Error)
			}
			runtime := taskruntime.NewStore(db).ForBusiness(id.String())
			before, err := runtime.Claim(t.Context(), []string{"operation.provider"}, "before", time.Minute)
			require.NoError(t, err)
			require.Nil(t, before)
			require.NoError(t, f.store.ScheduleOperationStep(t.Context(), "operation.provider", id))
			key := uuid.NewString()
			router := publicOperationRouter(actor, db)
			path, body := "/api/operations/"+id.String()+"/recover", fmt.Sprintf(`{"project_id":%q}`, pid)
			response := publicOperationRequest(t, router, http.MethodPost, path, body, key)
			require.Equal(t, 202, response.Code, response.Body.String())
			d := controlTaskDelivery(t, db, key)
			if scenario == "revoked" {
				require.NoError(t, db.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, actor.SessionID).Error)
			}
			errs := make(chan error, 2)
			var wg sync.WaitGroup
			for range 2 {
				wg.Go(func() {
					errs <- deliverRecoveryAsApp(t.Context(), db, d)
				})
			}
			wg.Wait()
			close(errs)
			for e := range errs {
				require.NoError(t, e)
			}
			claimed, err := runtime.Claim(t.Context(), []string{"operation.provider"}, "recovered", time.Minute)
			require.NoError(t, err)
			if scenario != "query" {
				require.Nil(t, claimed)
				require.Equal(t, 1, posts)
				require.Zero(t, gets)
				return
			}
			require.NotNil(t, claimed, "explicit recover must reopen the parked original query")
			require.Greater(t, claimed.Epoch, int64(7))
			require.Equal(t, 15, claimed.Attempt)
			require.ErrorIs(t, runtime.Complete(t.Context(), taskruntime.Job{Kind: "operation.provider", BusinessID: id.String(), Owner: "old-owner", Epoch: 7}), taskruntime.ErrLeaseLost)
			_, err = h["operation.provider"](t.Context(), *claimed)
			require.NoError(t, err)
			require.Equal(t, 1, posts)
			require.Equal(t, 1, gets)
			require.NoError(t, db.Exec(`UPDATE infra.task_work SET failed_at=now(),failures=6,owner=NULL,lease_until=NULL WHERE kind='operation.provider' AND business_id=?`, id.String()).Error)
			replay := publicOperationRequest(t, router, http.MethodPost, path, body, key)
			require.JSONEq(t, response.Body.String(), replay.Body.String())
			runControlTask(t, db, f.store, d)
			again, err := runtime.Claim(t.Context(), []string{"operation.provider"}, "replayed", time.Minute)
			require.NoError(t, err)
			require.Nil(t, again)
			newKey := uuid.NewString()
			response = publicOperationRequest(t, router, http.MethodPost, path, body, newKey)
			require.Equal(t, 202, response.Code, response.Body.String())
			runControlTask(t, db, f.store, controlTaskDelivery(t, db, newKey))
			again, err = runtime.Claim(t.Context(), []string{"operation.provider"}, "new-command", time.Minute)
			require.NoError(t, err)
			require.NotNil(t, again)
			require.Greater(t, again.Epoch, claimed.Epoch)
			require.ErrorIs(t, runtime.Complete(t.Context(), *claimed), taskruntime.ErrLeaseLost)
			require.Equal(t, 1, posts)
		})
	}
}

func TestExplicitRecoverReopensParkedDeliveryWithoutProviderDispatch(t *testing.T) {
	db := operationStoreDB(t)
	f := seedUnverifiedManagedFixture(t, db)
	id, pid := f.identity.OperationID, f.identity.ProjectID
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile SET capability='text.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, f.identity.ModelProfileVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text_to_text'],queue='provider',moderation='provider' WHERE id=?`, f.identity.ModelProfileVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE operation.operation SET capability='text.generate',mode='text_to_text' WHERE id=?`, id).Error)
	seedDispatchModeEvidence(t, db, f)
	claim, err := f.store.ClaimProviderDispatch(t.Context(), f.identity)
	require.NoError(t, err)
	require.True(t, claim.Claimed)
	results := application.NewManagedResults(&stageTestObjects{entries: map[string]stageTestObject{}}, f.store)
	require.NoError(t, results.Save(t.Context(), application.ManagedResult{Identity: f.identity, State: application.ManagedResultSucceeded, Text: "original archived response", Usage: json.RawMessage(`{"billable_requests":1}`)}))
	require.NoError(t, db.Exec(`UPDATE operation.operation SET status='ingesting' WHERE id=?`, id).Error)
	require.NoError(t, f.store.ScheduleOperationStep(t.Context(), "operation.ingest", id))
	require.NoError(t, db.Exec(`UPDATE infra.task_work SET failed_at=now(),failures=6,epoch=3,attempt=6 WHERE kind='operation.ingest' AND business_id=?`, id.String()).Error)
	var actor identityapp.Principal
	require.NoError(t, db.Raw(`SELECT u.id,u.org_id,u.role FROM identity."user" u JOIN operation.operation o ON o.confirmed_by=u.id WHERE o.id=?`, id).Scan(&actor).Error)
	actor = identityfixture.Session(t, db, actor)
	key := uuid.NewString()
	response := publicOperationRequest(t, publicOperationRouter(actor, db), http.MethodPost, "/api/operations/"+id.String()+"/recover", fmt.Sprintf(`{"project_id":%q}`, pid), key)
	require.Equal(t, 202, response.Code, response.Body.String())
	runControlTask(t, db, f.store, controlTaskDelivery(t, db, key))
	runtime := taskruntime.NewStore(db).ForBusiness(id.String())
	job, err := runtime.Claim(t.Context(), []string{"operation.ingest"}, "delivery", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job, "explicit recover must reopen parked original delivery")
	require.Greater(t, job.Epoch, int64(3))
	h := map[string]taskruntime.Handler{}
	operationflow.RegisterMedia(h, f.store, nil, managed.NewRoutedIngestActivities(db, nil, results, nil, f.store))
	_, err = h["operation.ingest"](t.Context(), *job)
	require.NoError(t, err)
	var outputs, submits int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM operation.operation_output WHERE operation_id=?`, id).Scan(&outputs).Error)
	require.NoError(t, db.Raw(`SELECT count(*) FROM operation.provider_call WHERE operation_id=? AND action='submit'`, id).Scan(&submits).Error)
	require.EqualValues(t, 1, outputs)
	require.EqualValues(t, 1, submits)
}

func deliverRecoveryAsApp(ctx context.Context, db *gorm.DB, d application.WorkflowControlDelivery) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
			return err
		}
		s := operationStore(tx)
		h := map[string]taskruntime.Handler{}
		operationflow.RegisterFlow(h, s, app.NewOperationFinalizer(tx, s, billingpg.NewStore(tx)), nil)
		raw, err := json.Marshal(d)
		if err != nil {
			return err
		}
		_, err = h["operation.control"](ctx, taskruntime.Job{BusinessID: d.EventID.String(), Reference: raw})
		return err
	})
}
