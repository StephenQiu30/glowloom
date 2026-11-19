package aigateway_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

type providerFunc func(context.Context, application.Dispatch, func(json.RawMessage) error) (domain.Receipt, error)

func (f providerFunc) Send(ctx context.Context, d application.Dispatch, emit func(json.RawMessage) error) (domain.Receipt, error) {
	return f(ctx, d, emit)
}

func TestRebuiltWorkbenchQuoteConfirmWorkerAndBills(t *testing.T) {
	db, store, model, a := fixture(t)
	pid := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'AI workbench','16:9','realistic')`, pid, a.OrgID, a.ID, a.ScopeID).Error)
	var sends atomic.Int64
	runtime := &aigateway.Runtime{DB: db, Provider: providerFunc(func(_ context.Context, d application.Dispatch, _ func(json.RawMessage) error) (domain.Receipt, error) {
		sends.Add(1)
		require.Equal(t, "creation", d.Call.Origin)
		require.Equal(t, pid, d.Call.Project)
		usage := domain.Usage{Input: 10, Output: 2}
		cost, err := d.Route.Purchase.Cost(usage)
		require.NoError(t, err)
		return domain.Receipt{Body: json.RawMessage(`{"model":"supplier-model","choices":[{"message":{"content":"reconstructed result"},"finish_reason":"stop"}]}`), Usage: usage, PurchaseMicros: cost}, nil
	})}
	creation := aigateway.Creation{Runtime: runtime, Authorize: fixtureAuthority}
	input := operationhttp.QuoteItemRequest{ModelKey: model.Name, Capability: "text.generate", Mode: "text_to_text", Prompt: "new code", Params: json.RawMessage(`{"max_tokens":100}`), OutputCount: 1}
	engine := gin.New()
	group := engine.Group("/api", func(c *gin.Context) { c.Set("principal", a) })
	creation.RegisterCreation(group)
	body, err := json.Marshal(input)
	require.NoError(t, err)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/projects/"+pid.String()+"/free-operations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "quote-once")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	require.Equal(t, http.StatusCreated, res.Code, res.Body.String())
	var quote operationhttp.QuoteResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &quote))
	require.NotEqual(t, uuid.Nil, quote.OperationID)
	require.NotNil(t, quote.ConfirmKey)
	require.EqualValues(t, 100000, quote.QuoteMicros)
	require.True(t, quote.Confirmable)
	require.Contains(t, res.Body.String(), `"confirmable":true`)
	replay, err := creation.Quote(t.Context(), a, pid, "quote-once", input)
	require.NoError(t, err)
	require.Equal(t, quote.OperationID, replay.OperationID)
	var held, count int64
	require.NoError(t, db.Raw("SELECT held FROM ai.account WHERE scope_id=?", a.ScopeID).Scan(&held).Error)
	require.Zero(t, held)
	require.Zero(t, sends.Load())
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			_, err := creation.Confirm(t.Context(), a, pid, quote.OperationID, "confirmed")
			require.NoError(t, err)
		})
	}
	wg.Wait()
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.hold WHERE request_id=?", quote.OperationID).Scan(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Raw("SELECT count(*) FROM infra.task_work WHERE kind='ai.execute' AND business_id=?", quote.OperationID.String()).Scan(&count).Error)
	require.EqualValues(t, 1, count)
	for range 2 {
		next, err := creation.Execute(t.Context(), taskruntime.Job{Kind: "ai.execute", BusinessID: quote.OperationID.String()})
		require.NoError(t, err)
		require.True(t, next.IsZero())
	}
	require.EqualValues(t, 1, sends.Load())
	task, err := creation.Task(t.Context(), a, pid, quote.OperationID)
	require.NoError(t, err)
	require.Equal(t, "completed", string(task.Status))
	require.Len(t, task.Outputs, 1)
	require.Equal(t, "reconstructed result", *task.Outputs[0].Text)
	bills, err := store.Bills(t.Context(), a.ScopeID)
	require.NoError(t, err)
	require.Len(t, bills, 1)
	require.Equal(t, quote.OperationID, bills[0].ID)
	require.EqualValues(t, 1, bills[0].Charged)
	input.Prompt = "different"
	_, err = creation.Quote(t.Context(), a, pid, "quote-once", input)
	require.ErrorIs(t, err, domain.ErrConflict)
	require.NoError(t, db.Raw("SELECT held FROM ai.account WHERE scope_id=?", a.ScopeID).Scan(&held).Error)
	require.Zero(t, held)
}

func TestCancelledSentRequestAndRevokedConsumedClaimRetainFundsWithoutResend(t *testing.T) {
	t.Run("client-cancel", func(t *testing.T) {
		db, store, m, a := fixture(t)
		c := call(t, m, a)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		started := make(chan struct{})
		var sends atomic.Int64
		service := application.New(store, providerFunc(func(ctx context.Context, _ application.Dispatch, _ func(json.RawMessage) error) (domain.Receipt, error) {
			sends.Add(1)
			close(started)
			<-ctx.Done()
			return domain.Receipt{}, ctx.Err()
		}))
		finished := make(chan error, 1)
		go func() { _, err := service.Execute(ctx, c, nil); finished <- err }()
		<-started
		cancel()
		require.ErrorIs(t, <-finished, domain.ErrUnknown)
		record, err := service.Execute(t.Context(), c, nil)
		require.NoError(t, err)
		require.Equal(t, "unknown", record.State)
		require.EqualValues(t, 1, sends.Load())
		require.Zero(t, record.Charged)
		require.Zero(t, record.Released)
		var held int64
		require.NoError(t, db.Raw("SELECT held FROM ai.account WHERE scope_id=?", a.ScopeID).Scan(&held).Error)
		require.Equal(t, c.Limit, held)
	})
	t.Run("revoked-after-consumed-claim", func(t *testing.T) {
		db, _, m, a := fixture(t)
		pid := entrypointProject(t, db, a)
		entrypointSession(t, db, a)
		var sends atomic.Int64
		runtime := &aigateway.Runtime{DB: db, Provider: providerFunc(func(context.Context, application.Dispatch, func(json.RawMessage) error) (domain.Receipt, error) {
			sends.Add(1)
			return domain.Receipt{}, domain.ErrUnknown
		})}
		creation := aigateway.Creation{Runtime: runtime}
		q, err := creation.Quote(t.Context(), a, pid, "consumed", entrypointTextInput(m))
		require.NoError(t, err)
		_, err = creation.Confirm(t.Context(), a, pid, q.OperationID, "confirm")
		require.NoError(t, err)
		store := runtime.Store(a)
		call, err := store.LoadQuote(t.Context(), a.ScopeID, q.OperationID)
		require.NoError(t, err)
		dispatch, err := store.ClaimQueued(t.Context(), call)
		require.NoError(t, err)
		require.True(t, dispatch.Send)
		require.NoError(t, db.Exec("UPDATE identity.user_session SET revoked_at=clock_timestamp() WHERE id=?", a.SessionID).Error)
		_, err = creation.Execute(t.Context(), taskruntime.Job{BusinessID: q.OperationID.String()})
		require.NoError(t, err)
		require.Zero(t, sends.Load())
		var state string
		require.NoError(t, db.Raw("SELECT state FROM ai.request WHERE id=?", q.OperationID).Scan(&state).Error)
		require.Equal(t, "unknown", state)
		var count int64
		require.NoError(t, db.Raw("SELECT count(*) FROM ai.attempt WHERE request_id=? AND state='unknown'", q.OperationID).Scan(&count).Error)
		require.EqualValues(t, 1, count)
		var held int64
		require.NoError(t, db.Raw("SELECT held FROM ai.account WHERE scope_id=?", a.ScopeID).Scan(&held).Error)
		require.Positive(t, held)
	})
}
