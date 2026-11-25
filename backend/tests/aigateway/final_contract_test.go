package aigateway_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	gatewayhttp "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/http"
	pg "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/provider"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/adapter/http"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

func textProvider(_ context.Context, d application.Dispatch, _ func(json.RawMessage) error) (domain.Receipt, error) {
	u := domain.Usage{Input: 1, Output: 1}
	cost, err := d.Route.Purchase.Cost(u)
	return domain.Receipt{Body: json.RawMessage(`{"model":"supplier-model","choices":[{"message":{"content":"finished text"},"finish_reason":"stop"}]}`), Usage: u, PurchaseMicros: cost}, err
}
func creationRouter(c aigateway.Creation, a identity.Principal) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	c.RegisterCreation(router.Group("/api", func(ctx *gin.Context) { ctx.Set("principal", a) }))
	return router
}
func controlRequest(t *testing.T, router *gin.Engine, p, id uuid.UUID, action, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), "POST", "/api/operations/"+id.String()+"/"+action, strings.NewReader(fmt.Sprintf(`{"project_id":%q}`, p.String())))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	out := httptest.NewRecorder()
	router.ServeHTTP(out, req)
	return out
}
func TestFinalSynchronousCrashHasDurableUnknownRecovery(t *testing.T) {
	for _, lateReceipt := range []bool{false, true} {
		t.Run(fmt.Sprint(lateReceipt), func(t *testing.T) {
			t.Parallel()
			db, s, m, a := fixture(t)
			c := call(t, m, a)
			c.Deadline = time.Now().Add(100 * time.Millisecond)
			d, err := s.Admit(t.Context(), c)
			require.NoError(t, err)
			var jobs int64
			require.NoError(t, db.Raw("SELECT count(*) FROM infra.task_work WHERE kind='ai.recover' AND business_id=?", c.ID.String()).Scan(&jobs).Error)
			require.EqualValues(t, 1, jobs)
			handlers := map[string]taskruntime.Handler{}
			(&aigateway.Runtime{DB: db}).RegisterWorker(handlers)
			require.NotNil(t, handlers["ai.recover"])
			// The original sender may still be alive: the wakeup must not invalidate its bounded window.
			next, err := handlers["ai.recover"](t.Context(), taskruntime.Job{BusinessID: c.ID.String()})
			require.NoError(t, err)
			require.True(t, next.After(time.Now()))
			time.Sleep(time.Until(c.Deadline.Add(16 * time.Second)))
			c.Deadline = time.Now().Add(time.Minute)
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			worker := taskruntime.Worker{Store: taskruntime.NewStore(db).ForBusiness(c.ID.String()), Handlers: map[string]taskruntime.Handler{"ai.recover": handlers["ai.recover"]}, Concurrency: 1, Lease: time.Second, Poll: time.Millisecond}
			go func() { done <- worker.Run(ctx) }()
			require.Eventually(t, func() bool {
				var state string
				require.NoError(t, db.Raw("SELECT state FROM ai.request WHERE id=?", c.ID).Scan(&state).Error)
				return state == "unknown"
			}, time.Second, 5*time.Millisecond)
			cancel()
			require.NoError(t, <-done)
			sends := 0
			res, err := application.New(pg.New(db, a, fixtureAuthority), providerFunc(func(context.Context, application.Dispatch, func(json.RawMessage) error) (domain.Receipt, error) {
				sends++
				return domain.Receipt{}, nil
			})).Execute(t.Context(), c, nil)
			require.NoError(t, err)
			require.Equal(t, "unknown", res.State)
			require.Zero(t, sends)
			require.Zero(t, res.Released)
			if lateReceipt {
				receipt, e := textProvider(t.Context(), d, nil)
				require.NoError(t, e)
				require.NoError(t, s.SaveReceipt(t.Context(), c.ID, d.Attempt, receipt))
				res, err = s.Settle(t.Context(), c.ID)
				require.NoError(t, err)
				require.Equal(t, "settled", res.State)
			} else {
				res, err = s.Reconcile(t.Context(), pg.Reconciliation{Request: c.ID, Attempt: d.Attempt, Reference: "original-statement", Reason: "original supplier confirms no charge", NotCharged: true})
				require.NoError(t, err)
				require.Equal(t, "released", res.State)
			}
			var attempts int64
			require.NoError(t, db.Raw("SELECT count(*) FROM ai.attempt WHERE request_id=?", c.ID).Scan(&attempts).Error)
			require.EqualValues(t, 1, attempts)
		})
	}
}
func TestFinalRawReceiptRequiresConsumerOrLiveTeamAdmin(t *testing.T) {
	db, _, m, a := fixture(t)
	_, _, _, member := fixture(t)
	entrypointProject(t, db, a)
	entrypointSession(t, db, a)
	entrypointProject(t, db, member)
	entrypointSession(t, db, member)
	team, sid := uuid.New(), uuid.New()
	require.NoError(t, db.Exec("INSERT INTO workspace.team(id,name,created_by) VALUES(?,'Private receipts',?)", team, a.ID).Error)
	require.NoError(t, db.Exec("INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)", sid, team, a.OrgID).Error)
	require.NoError(t, db.Exec("INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',clock_timestamp(),'created'),(?,?,'member','active',clock_timestamp(),'created')", team, a.ID, team, member.ID).Error)
	a.ScopeID = sid
	member.ScopeID = sid
	member.OrgID = a.OrgID
	s := pg.New(db, a, aigateway.Authority)
	require.NoError(t, s.Grant(t.Context(), sid, "fund", 100))
	c := call(t, m, a)
	res, err := application.New(s, providerFunc(textProvider)).Execute(t.Context(), c, nil)
	require.NoError(t, err)
	read := func(who identity.Principal) int {
		router := gin.New()
		gatewayhttp.New(db, aigateway.Authority, nil, nil).Register(router.Group("/api"), router.Group("/api", func(ctx *gin.Context) { ctx.Set("principal", who) }))
		req := httptest.NewRequestWithContext(t.Context(), "GET", "/api/ai/requests/"+res.ID.String(), nil)
		req.Header.Set("X-Creation-Scope", sid.String())
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		return out.Code
	}
	require.Equal(t, 403, read(member))
	require.Equal(t, 200, read(a))
	require.NoError(t, db.Exec("UPDATE workspace.team_member SET role='team_admin' WHERE team_id=? AND user_id=?", team, member.ID).Error)
	require.Equal(t, 200, read(member))
	require.NoError(t, db.Exec("UPDATE workspace.team_member SET role='member' WHERE team_id=? AND user_id=?", team, member.ID).Error)
	require.Equal(t, 403, read(member))
	p := entrypointProject(t, db, a)
	creation := aigateway.Creation{Runtime: &aigateway.Runtime{DB: db, Provider: providerFunc(textProvider)}}
	items := []operationhttp.QuoteItemRequest{entrypointTextInput(m)}
	batch, err := creation.BatchQuote(t.Context(), a, p, "team-key", items)
	require.NoError(t, err)
	require.NotNil(t, batch.BatchID)
	_, err = creation.BatchQuote(t.Context(), member, p, "team-key", items)
	require.ErrorIs(t, err, domain.ErrConflict)
	quote, err := creation.Quote(t.Context(), a, p, "shared-task", entrypointTextInput(m))
	require.NoError(t, err)
	_, err = creation.Confirm(t.Context(), a, p, quote.OperationID, "confirm")
	require.NoError(t, err)
	_, err = creation.Execute(t.Context(), taskruntime.Job{BusinessID: quote.OperationID.String()})
	require.NoError(t, err)
	shared, err := creation.Task(t.Context(), member, p, quote.OperationID)
	require.NoError(t, err)
	require.Equal(t, "finished text", *shared.Outputs[0].Text)
	_, err = pg.New(db, member, aigateway.Authority).Read(t.Context(), sid, quote.OperationID)
	require.ErrorIs(t, err, domain.ErrForbidden)
}
func TestFinalControlReceiptReplaysAndRejectsChangedIntent(t *testing.T) {
	db, _, m, a := fixture(t)
	p := entrypointProject(t, db, a)
	c := aigateway.Creation{Runtime: &aigateway.Runtime{DB: db, Provider: providerFunc(textProvider)}, Authorize: fixtureAuthority}
	q, err := c.Quote(t.Context(), a, p, uuid.NewString(), entrypointTextInput(m))
	require.NoError(t, err)
	_, err = c.Confirm(t.Context(), a, p, q.OperationID, "confirm")
	require.NoError(t, err)
	router := creationRouter(c, a)
	key := uuid.NewString()
	first := controlRequest(t, router, p, q.OperationID, "cancel", key)
	require.Equal(t, 202, first.Code, first.Body.String())
	var receipt operation.WorkflowControlResult
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &receipt))
	require.Equal(t, "operation", receipt.TargetType)
	require.True(t, receipt.Accepted)
	again := controlRequest(t, router, p, q.OperationID, "cancel", key)
	require.Equal(t, 202, again.Code, again.Body.String())
	require.JSONEq(t, first.Body.String(), again.Body.String())
	changed := controlRequest(t, router, p, q.OperationID, "recover", key)
	require.Equal(t, 409, changed.Code, changed.Body.String())
	final := controlRequest(t, router, p, q.OperationID, "cancel", uuid.NewString())
	require.Equal(t, 202, final.Code)
	require.NoError(t, json.Unmarshal(final.Body.Bytes(), &receipt))
	require.False(t, receipt.Accepted)
}
func TestFinalBatchReplayBindsActorAndProject(t *testing.T) {
	db, _, m, a := fixture(t)
	p1 := entrypointProject(t, db, a)
	p2 := entrypointProject(t, db, a)
	c := aigateway.Creation{Runtime: &aigateway.Runtime{DB: db}, Authorize: fixtureAuthority}
	items := []operationhttp.QuoteItemRequest{entrypointTextInput(m)}
	first, err := c.BatchQuote(t.Context(), a, p1, "same-key", items)
	require.NoError(t, err)
	again, err := c.BatchQuote(t.Context(), a, p1, "same-key", items)
	require.NoError(t, err)
	require.Equal(t, first.BatchID, again.BatchID)
	_, err = c.BatchQuote(t.Context(), a, p2, "same-key", items)
	require.ErrorIs(t, err, domain.ErrConflict)
	other := a
	other.ID = uuid.New()
	_, err = c.BatchQuote(t.Context(), other, p1, "same-key", items)
	require.ErrorIs(t, err, domain.ErrForbidden)
}
func TestFinalCacheAliasPresence(t *testing.T) {
	for _, v := range []struct {
		name, fields string
		unknown      bool
		cache        int64
	}{
		{"zero-conflict", `"prompt_tokens_details":{"cached_tokens":0},"prompt_cache_hit_tokens":5`, true, 0},
		{"nonzero-conflict", `"prompt_tokens_details":{"cached_tokens":5},"prompt_cache_hit_tokens":0`, true, 0},
		{"zero-agrees", `"prompt_tokens_details":{"cached_tokens":0},"prompt_cache_hit_tokens":0`, false, 0},
		{"agrees", `"prompt_tokens_details":{"cached_tokens":5},"prompt_cache_hit_tokens":5`, false, 5},
		{"alias-only", `"prompt_cache_hit_tokens":5`, false, 5},
		{"empty-details", `"prompt_tokens_details":{},"prompt_cache_hit_tokens":5`, false, 5},
	} {
		t.Run(v.name, func(t *testing.T) {
			u, err := provider.Usage("chat", []byte(`{"usage":{"prompt_tokens":10,"completion_tokens":1,`+v.fields+`}}`))
			if v.unknown {
				require.ErrorIs(t, err, domain.ErrUnknown)
			} else {
				require.NoError(t, err)
				require.Equal(t, v.cache, u.CacheRead)
			}
		})
	}
}

type unavailableArchiver struct{}

func (unavailableArchiver) ArchiveGatewaySource(context.Context, mediaapp.SourceOperation, uuid.UUID, *mediaapp.Downloaded) (media.MediaAsset, []media.Rendition, error) {
	return media.MediaAsset{}, nil, mediaapp.ErrUnavailable
}

func TestFinalParkedDeliveryRecoversThroughHTTPAndRealWorker(t *testing.T) {
	db, s, _, a := fixture(t)
	p := entrypointProject(t, db, a)
	m := mediaModel(t, s, "image.generate", "provider_success_v1")
	body := pngBytes(t)
	sends := 0
	r := &aigateway.Runtime{DB: db, Objects: fixtureMediaStorage(t, db), MediaArchive: unavailableArchiver{}, Provider: providerFunc(func(_ context.Context, d application.Dispatch, _ func(json.RawMessage) error) (domain.Receipt, error) {
		sends++
		raw, err := json.Marshal(operation.GenerationResult{Outcome: operation.ProviderOutcomeCompleted, State: operation.ProviderTaskSucceeded, Images: []operation.GeneratedImage{{Bytes: body, MediaType: "image/png"}}})
		require.NoError(t, err)
		cost, e := d.Route.Purchase.Cost(domain.Usage{})
		return domain.Receipt{Body: raw, PurchaseMicros: cost}, e
	})}
	c := aigateway.Creation{Runtime: r, Authorize: fixtureAuthority}
	q, err := c.Quote(t.Context(), a, p, uuid.NewString(), operationhttp.QuoteItemRequest{ModelKey: m.Name, Capability: m.Capability, Mode: "text2image", Prompt: "fixture", Params: json.RawMessage(`{}`), OutputCount: 1})
	require.NoError(t, err)
	_, err = c.Confirm(t.Context(), a, p, q.OperationID, "confirm")
	require.NoError(t, err)
	_, err = c.Execute(t.Context(), taskruntime.Job{BusinessID: q.OperationID.String()})
	require.NoError(t, err)
	handlers := map[string]taskruntime.Handler{}
	r.RegisterWorker(handlers)
	run := func() (context.CancelFunc, chan error) {
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		worker := taskruntime.Worker{Store: taskruntime.NewStore(db).ForBusiness(q.OperationID.String()), Handlers: map[string]taskruntime.Handler{"ai.deliver": handlers["ai.deliver"]}, Concurrency: 1, Lease: time.Second, Poll: 5 * time.Millisecond}
		go func() { done <- worker.Run(ctx) }()
		return cancel, done
	}
	cancel, done := run()
	require.Eventually(t, func() bool {
		var row struct {
			Failures int
			Failed   bool
		}
		require.NoError(t, db.Raw("SELECT failures,failed_at IS NOT NULL AS failed FROM infra.task_work WHERE kind='ai.deliver' AND business_id=?", q.OperationID.String()).Scan(&row).Error)
		// Accelerate only test backoff; the production Worker performs all six failures.
		require.NoError(t, db.Exec("UPDATE infra.task_work SET next_run_at=clock_timestamp() WHERE kind='ai.deliver' AND business_id=? AND owner IS NULL AND failed_at IS NULL", q.OperationID.String()).Error)
		return row.Failed && row.Failures == 6
	}, 10*time.Second, 20*time.Millisecond)
	cancel()
	require.NoError(t, <-done)
	task, err := c.Task(t.Context(), a, p, q.OperationID)
	require.NoError(t, err)
	require.Equal(t, "recoverable", task.BillingOutcome.DeliveryState)
	router := creationRouter(c, a)
	require.NoError(t, db.Exec("UPDATE infra.task_work SET owner='active-fixture',lease_until=clock_timestamp()+interval '1 minute' WHERE kind='ai.deliver' AND business_id=?", q.OperationID.String()).Error)
	locked := controlRequest(t, router, p, q.OperationID, "recover", uuid.NewString())
	require.Equal(t, 202, locked.Code)
	var receipt operation.WorkflowControlResult
	require.NoError(t, json.Unmarshal(locked.Body.Bytes(), &receipt))
	require.False(t, receipt.Accepted)
	require.NoError(t, db.Exec("UPDATE infra.task_work SET owner=NULL,lease_until=NULL WHERE kind='ai.deliver' AND business_id=?", q.OperationID.String()).Error)
	key := uuid.NewString()
	out := controlRequest(t, router, p, q.OperationID, "recover", key)
	require.Equal(t, 202, out.Code, out.Body.String())
	require.NoError(t, json.Unmarshal(out.Body.Bytes(), &receipt))
	require.True(t, receipt.Accepted)
	again := controlRequest(t, router, p, q.OperationID, "recover", key)
	require.JSONEq(t, out.Body.String(), again.Body.String())
	objects := &durableBytes{t: t, db: db, routes: pgmedia.NewStorageRoutes(db), files: map[string][]byte{}}
	r.MediaArchive = mediaapp.NewGeneratedOutputArchiver(objects.routes, objects, mediaflow.FFProber{}, mediaflow.FFRenderer{})
	cancel, done = run()
	require.Eventually(t, func() bool {
		var n int64
		require.NoError(t, db.Raw("SELECT count(*) FROM infra.task_work WHERE kind='ai.deliver' AND business_id=? AND completed_at IS NOT NULL", q.OperationID.String()).Scan(&n).Error)
		return n == 1
	}, 10*time.Second, 20*time.Millisecond)
	cancel()
	require.NoError(t, <-done)
	task, err = c.Task(t.Context(), a, p, q.OperationID)
	require.NoError(t, err)
	require.Equal(t, "ready", task.BillingOutcome.DeliveryState)
	require.Equal(t, 1, sends)
	var original struct{ ObjectKey string }
	require.NoError(t, db.Raw("SELECT object_key FROM media.media_asset WHERE source_ai_request_id=?", q.OperationID).Scan(&original).Error)
	require.Equal(t, body, objects.files[original.ObjectKey])
	var n int64
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.money_event WHERE request_id=? AND kind='settle'", q.OperationID).Scan(&n).Error)
	require.EqualValues(t, 1, n)
}

func TestFinalTaskProjectionAndCompleteTextDownload(t *testing.T) {
	for _, length := range []int{10, 70000, 0} {
		t.Run(fmt.Sprint(length), func(t *testing.T) {
			db, _, m, a := fixture(t)
			p := entrypointProject(t, db, a)
			full := strings.Repeat("光", length)
			if length == 0 {
				full = strings.Repeat(" ", 70000) + "可用正文"
			}
			c := aigateway.Creation{Runtime: &aigateway.Runtime{DB: db, Provider: providerFunc(func(ctx context.Context, d application.Dispatch, emit func(json.RawMessage) error) (domain.Receipt, error) {
				r, e := textProvider(ctx, d, emit)
				r.Body = canonicalTextBody(full)
				return r, e
			})}, Authorize: fixtureAuthority}
			q, err := c.Quote(t.Context(), a, p, uuid.NewString(), entrypointTextInput(m))
			require.NoError(t, err)
			_, err = c.Confirm(t.Context(), a, p, q.OperationID, "confirm")
			require.NoError(t, err)
			queued, err := c.Task(t.Context(), a, p, q.OperationID)
			require.NoError(t, err)
			require.Equal(t, "submitted", string(queued.Status))
			_, err = c.Execute(t.Context(), taskruntime.Job{BusinessID: q.OperationID.String()})
			require.NoError(t, err)
			task, err := c.Task(t.Context(), a, p, q.OperationID)
			require.NoError(t, err)
			require.Equal(t, "completed", string(task.Status))
			require.Len(t, task.Outputs, 1)
			out := task.Outputs[0]
			require.Equal(t, "json", out.Kind)
			require.Equal(t, "skipped", out.ModerationStatus)
			require.Equal(t, "unreviewed_text_v1", out.OutputPolicy)
			require.LessOrEqual(t, len(*out.Text), 65536)
			require.True(t, utf8.ValidString(*out.Text))
			require.NotEmpty(t, strings.TrimSpace(*out.Text))
			require.Equal(t, len(full) > 65536, out.TextTruncated)
			router := creationRouter(c, a)
			req := httptest.NewRequestWithContext(t.Context(), "GET", "/api/operations/"+q.OperationID.String()+"/text?project_id="+p.String(), nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, 200, w.Code, w.Body.String())
			require.Equal(t, full, w.Body.String())
			req = httptest.NewRequestWithContext(t.Context(), "GET", "/api/operations/"+q.OperationID.String()+"/text?project_id="+uuid.NewString(), nil)
			w = httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, 403, w.Code)
			if file := os.Getenv("GLOWLOOM_TEST_AI_TASK_FIXTURE"); file != "" && length == 10 {
				data, e := json.Marshal(map[string]any{"project": p, "queued": queued, "completed": task})
				require.NoError(t, e)
				require.NoError(t, os.WriteFile(file, data, 0600))
			}
		})
	}
}
func canonicalTextBody(text string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": text}, "finish_reason": "stop"}}})
	return b
}

func TestFinalKnownEmptyOrRefusedTextKeepsChargeAndUsableTaskContract(t *testing.T) {
	for _, refused := range []bool{false, true} {
		t.Run(fmt.Sprint(refused), func(t *testing.T) {
			db, _, m, a := fixture(t)
			p := entrypointProject(t, db, a)
			c := aigateway.Creation{Runtime: &aigateway.Runtime{DB: db, Provider: providerFunc(func(ctx context.Context, d application.Dispatch, emit func(json.RawMessage) error) (domain.Receipt, error) {
				r, e := textProvider(ctx, d, emit)
				r.Body = canonicalTextBody("")
				if refused {
					r.Body = json.RawMessage(`{"choices":[{"message":{"content":null,"refusal":"cannot comply"},"finish_reason":"stop"}]}`)
				}
				return r, e
			})}, Authorize: fixtureAuthority}
			q, err := c.Quote(t.Context(), a, p, uuid.NewString(), entrypointTextInput(m))
			require.NoError(t, err)
			_, err = c.Confirm(t.Context(), a, p, q.OperationID, "confirm")
			require.NoError(t, err)
			_, err = c.Execute(t.Context(), taskruntime.Job{BusinessID: q.OperationID.String()})
			require.NoError(t, err)
			task, err := c.Task(t.Context(), a, p, q.OperationID)
			require.NoError(t, err)
			require.Equal(t, "failed", string(task.Status))
			require.Empty(t, task.Outputs)
			require.Equal(t, "settled", task.BillingOutcome.ChargeState)
			require.EqualValues(t, 100000, *task.BillingOutcome.ChargedMicros)
			require.Equal(t, "unrecoverable", task.BillingOutcome.DeliveryState)
			require.False(t, task.BillingOutcome.ReconciliationRequired)
			code := "provider_empty_text"
			if refused {
				code = "provider_output_refused"
			}
			require.Equal(t, code, *task.FailureCode)
		})
	}
}
