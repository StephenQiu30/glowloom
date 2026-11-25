package aigateway_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	cataloghttp "github.com/StephenQiu30/glowloom-server/backend/internal/execution/catalog/adapter/http"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/execution/catalog/application"
	gatewayhttp "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/http"
	pg "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/provider"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

func TestOnboardingModelDraftKeepsLiveAlias(t *testing.T) {
	db, store, live, actor := fixture(t)
	engine := gin.New()
	api := engine.Group("/api")
	session := api.Group("", func(c *gin.Context) { c.Set("principal", actor) })
	gatewayhttp.New(db, fixtureAuthority, nil, nil).Register(api, session)

	next := live
	next.ID = uuid.New()
	body, err := json.Marshal(next)
	require.NoError(t, err)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/ai/admin/models", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	current, err := store.Model(t.Context(), live.Name, live.Protocol)
	require.NoError(t, err)
	require.Equal(t, live.ID, current.ID, "preparation must leave the current version available")
}

type onboardingSealer struct{}

func (onboardingSealer) KeyID() string { return "fixture" }
func (onboardingSealer) Seal(uuid.UUID, uuid.UUID, json.RawMessage) ([]byte, error) {
	return []byte("sealed-test-value"), nil
}

func TestOnboardingChargesOnceAndSwitchesOnlyOnEvidence(t *testing.T) {
	db, legacy, live, actor := fixture(t)
	entrypointProject(t, db, actor)
	entrypointSession(t, db, actor)
	store := pg.New(db, actor, aigateway.Authority)

	var sends atomic.Int64
	fail := atomic.Bool{}
	upstream := provider.New(keyFixture{}, &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		sends.Add(1)
		require.Equal(t, "Bearer fixture-only", r.Header.Get("Authorization"))
		var input struct {
			Model  string
			Stream bool
			Tools  []json.RawMessage
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
		require.Equal(t, "actual-supplier", input.Model)
		header := http.Header{"Content-Type": {"application/json"}}
		body := `{"model":"actual-supplier","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":3}}`
		if len(input.Tools) > 0 {
			body = strings.ReplaceAll(response("chat", input.Stream, false), `"name":"read"`, `"name":"verify"`)
		} else if input.Stream {
			body = "data: {\"model\":\"actual-supplier\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"OK\"},\"finish_reason\":null}]}\n\n" +
				"data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
				"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":3}}\n\n" +
				"data: [DONE]\n\n"
		}
		status := http.StatusOK
		if fail.Load() {
			status, body = http.StatusUnauthorized, `{"error":{"message":"supplier error"}}`
		}
		return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	engine := gin.New()
	api := engine.Group("/api")
	session := api.Group("", func(c *gin.Context) { c.Set("principal", actor) })
	gatewayhttp.New(db, aigateway.Authority, upstream, onboardingSealer{}).Register(api, session)
	request := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequestWithContext(t.Context(), method, "/api/ai/admin/"+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		result := httptest.NewRecorder()
		engine.ServeHTTP(result, req)
		return result
	}

	next := live
	next.ID = uuid.New()
	w := request(http.MethodPost, "models", next)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	in := application.Onboarding{
		Route: domain.Route{ID: uuid.New(), ModelID: next.ID, Supplier: uuid.New(), Group: uuid.NewString(), Protocol: "chat", BaseURL: "https://8.8.8.8/v1", UpstreamModel: "actual-supplier", Purchase: live.Price},
		Scope: actor.ScopeID, Concurrency: 2, RPM: 10000,
	}
	require.Equal(t, http.StatusBadRequest, request(http.MethodPost, "routes", map[string]any{"route": in.Route, "key": "not-allowed-here"}).Code)
	invalid := in
	invalid.Route.Priority = -1
	require.Equal(t, http.StatusBadRequest, request(http.MethodPost, "routes", invalid).Code)
	w = request(http.MethodPost, "routes", in)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var review pg.OnboardingReview
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &review))
	require.Equal(t, "prepared", review.State)
	require.Len(t, review.Plan.Probes, 4)
	require.Zero(t, sends.Load())
	ordinary := call(t, next, actor)
	_, err := legacy.Admit(t.Context(), ordinary)
	require.ErrorIs(t, err, domain.ErrUnavailable)

	path := "onboarding/" + in.Route.ID.String()
	firstPath := path
	approval := map[string]any{"key": "synthetic-api-key", "credits": review.Plan.Credits, "purchase_micros": review.Plan.PurchaseMicros}
	badApproval := map[string]any{"key": "synthetic-api-key", "credits": review.Plan.Credits - 1, "purchase_micros": review.Plan.PurchaseMicros}
	require.Equal(t, http.StatusPaymentRequired, request(http.MethodPost, path+"/verify", badApproval).Code)
	require.Zero(t, sends.Load())
	for range 2 {
		w = request(http.MethodPost, path+"/verify", approval)
		require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
		require.NotContains(t, w.Body.String(), "synthetic-api-key")
	}
	current, err := legacy.Model(t.Context(), live.Name, live.Protocol)
	require.NoError(t, err)
	require.Equal(t, live.ID, current.ID)

	runtime := &aigateway.Runtime{DB: db, Provider: upstream}
	handlers := map[string]taskruntime.Handler{}
	runtime.RegisterWorker(handlers)
	job := taskruntime.Job{Kind: pg.OnboardingKind, BusinessID: in.Route.ID.String()}
	for range 6 {
		_, err := handlers[pg.OnboardingKind](t.Context(), job)
		require.NoError(t, err)
	}
	require.EqualValues(t, 4, sends.Load())
	current, err = legacy.Model(t.Context(), live.Name, live.Protocol)
	require.NoError(t, err)
	require.Equal(t, next.ID, current.ID)
	review, err = store.ReviewOnboarding(t.Context(), in.Route.ID)
	require.NoError(t, err)
	require.Equal(t, "active", review.State)
	require.Len(t, review.Probes, 4)
	var settled int
	require.NoError(t, db.Raw(`SELECT count(*) FROM ai.money_event e JOIN ai.request r ON r.id=e.request_id WHERE r.run_id=? AND e.kind='settle'`, in.Route.ID).Row().Scan(&settled))
	require.Equal(t, 4, settled)

	// A later candidate can stay indeterminate without unpublishing the active version.
	later := next
	later.ID = uuid.New()
	require.Equal(t, http.StatusCreated, request(http.MethodPost, "models", later).Code)
	in.Route.ID, in.Route.ModelID = uuid.New(), later.ID
	w = request(http.MethodPost, "routes", in)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	path = "onboarding/" + in.Route.ID.String()
	require.Equal(t, http.StatusAccepted, request(http.MethodPost, path+"/verify", approval).Code)
	fail.Store(true)
	job.BusinessID = in.Route.ID.String()
	for range 2 {
		_, err := handlers[pg.OnboardingKind](t.Context(), job)
		require.NoError(t, err)
	}
	require.EqualValues(t, 5, sends.Load(), "unknown provider response must never cause a second submission")
	current, err = legacy.Model(t.Context(), live.Name, live.Protocol)
	require.NoError(t, err)
	require.Equal(t, next.ID, current.ID)
	review, err = store.ReviewOnboarding(t.Context(), in.Route.ID)
	require.NoError(t, err)
	require.Equal(t, "verifying", review.State)
	require.Equal(t, "unknown", review.Probes[0].State)

	require.Equal(t, http.StatusNoContent, request(http.MethodPost, firstPath+"/rollback", nil).Code)
	current, err = legacy.Model(t.Context(), live.Name, live.Protocol)
	require.NoError(t, err)
	require.Equal(t, live.ID, current.ID)
}

func TestOnboardingCompetingCandidatesAndTransactionFailure(t *testing.T) {
	db, oldStore, model, actor := fixture(t)
	entrypointProject(t, db, actor)
	entrypointSession(t, db, actor)
	store := pg.New(db, actor, aigateway.Authority)
	prepare := func() application.Onboarding {
		t.Helper()
		next := model
		next.ID = uuid.New()
		require.NoError(t, store.StageModel(t.Context(), next))
		in := application.Onboarding{
			Route: domain.Route{ID: uuid.New(), ModelID: next.ID, Supplier: uuid.New(), Group: uuid.NewString(), Protocol: "chat", BaseURL: "https://8.8.8.8/v1", UpstreamModel: "actual-supplier", Purchase: model.Price},
			Scope: actor.ScopeID, Concurrency: 2, RPM: 1000,
		}
		require.NoError(t, store.PrepareOnboarding(t.Context(), in))
		review, err := store.ReviewOnboarding(t.Context(), in.Route.ID)
		require.NoError(t, err)
		require.NoError(t, store.ConfirmOnboarding(t.Context(), in.Route.ID, review.Plan.Credits, review.Plan.PurchaseMicros,
			func(uuid.UUID, uuid.UUID) ([]byte, string, error) { return []byte("sealed fixture"), "fixture", nil }))
		work, err := store.OnboardingWork(t.Context(), in.Route.ID)
		require.NoError(t, err)
		validation := store.WithOnboarding(in.Route.ID, nil)
		for index, probe := range work.Plan.Probes {
			call, err := application.OnboardingCall(work.Input, next, actor.ID, probe, index)
			require.NoError(t, err)
			dispatched, err := validation.Admit(t.Context(), call)
			require.NoError(t, err)
			require.True(t, dispatched.Send)
			body := json.RawMessage(`{"choices":[{"message":{"content":"OK"}}]}`)
			if probe.Tool {
				body = json.RawMessage(`{"choices":[{"message":{"tool_calls":[{"function":{"name":"verify"}}]}}]}`)
			}
			usage := domain.Usage{Input: 12, Output: 3}
			micros, err := model.Price.Cost(usage)
			require.NoError(t, err)
			require.NoError(t, store.SaveReceipt(t.Context(), call.ID, dispatched.Attempt, domain.Receipt{Body: body, Usage: usage, PurchaseMicros: micros}))
			_, err = store.Settle(t.Context(), call.ID)
			require.NoError(t, err)
		}
		return in
	}
	a, b := prepare(), prepare()
	abort := errors.New("abort enclosing publish transaction")
	require.ErrorIs(t, db.Transaction(func(tx *gorm.DB) error {
		if err := pg.New(tx, actor, aigateway.Authority).PublishVerified(t.Context(), a.Route.ID); err != nil {
			return err
		}
		return abort
	}), abort)
	current, err := oldStore.Model(t.Context(), model.Name, model.Protocol)
	require.NoError(t, err)
	require.Equal(t, model.ID, current.ID)

	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, candidate := range []application.Onboarding{a, b} {
		wg.Go(func() { errs <- store.PublishVerified(t.Context(), candidate.Route.ID) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var winner pg.OnboardingReview
	states := []string{}
	for _, candidate := range []application.Onboarding{a, b} {
		review, err := store.ReviewOnboarding(t.Context(), candidate.Route.ID)
		require.NoError(t, err)
		states = append(states, review.State)
		if review.State == "active" {
			winner = review
		}
	}
	require.ElementsMatch(t, []string{"active", "stale"}, states)
	current, err = oldStore.Model(t.Context(), model.Name, model.Protocol)
	require.NoError(t, err)
	require.Equal(t, winner.ModelID, current.ID)
	var dispatches int
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.route WHERE model_id=? AND enabled", winner.ModelID).Row().Scan(&dispatches))
	require.Equal(t, 1, dispatches)
}

func TestOnboardingMediaWaitsForOriginalPollAndDelivery(t *testing.T) {
	db, legacy, _, actor := fixture(t)
	project := entrypointProject(t, db, actor)
	entrypointSession(t, db, actor)
	store := pg.New(db, actor, aigateway.Authority)
	live := mediaModel(t, legacy, "image.generate", "provider_success_v1")
	next := live
	next.ID = uuid.New()
	require.NoError(t, store.StageModel(t.Context(), next))
	var old struct{ Document []byte }
	require.NoError(t, db.Raw("SELECT document FROM ai.route WHERE model_id=?", live.ID).Scan(&old).Error)
	var route domain.Route
	require.NoError(t, json.Unmarshal(old.Document, &route))
	route.ID, route.ModelID = uuid.New(), next.ID
	mediaInput := domain.MediaInput{Model: next.Name, Capability: next.Capability, Mode: next.Modes[0], Prompt: "synthetic image", OutputCount: 1}
	raw, err := json.Marshal(mediaInput)
	require.NoError(t, err)
	in := application.Onboarding{Route: route, Scope: actor.ScopeID, Project: project, MediaTests: []json.RawMessage{raw}, Concurrency: 2, RPM: 10000}
	require.NoError(t, store.PrepareOnboarding(t.Context(), in))
	review, err := store.ReviewOnboarding(t.Context(), route.ID)
	require.NoError(t, err)
	require.NoError(t, store.ConfirmOnboarding(t.Context(), route.ID, review.Plan.Credits, review.Plan.PurchaseMicros,
		func(uuid.UUID, uuid.UUID) ([]byte, string, error) { return []byte("sealed fixture"), "fixture", nil }))

	objects := &durableBytes{t: t, db: db, routes: pgmedia.NewStorageRoutes(db), files: map[string][]byte{}}
	runtime := &aigateway.Runtime{DB: db, Objects: fixtureMediaStorage(t, db), MediaArchive: mediaapp.NewGeneratedOutputArchiver(objects.routes, objects, mediaflow.FFProber{}, mediaflow.FFRenderer{})}
	sends, polls := 0, 0
	var originalCredential uuid.UUID
	runtime.Provider = providerFunc(func(_ context.Context, d application.Dispatch, _ func(json.RawMessage) error) (domain.Receipt, error) {
		require.Equal(t, route.ID, d.Route.ID)
		if d.PollTask == "" {
			sends++
			originalCredential = d.Route.Credential
			return domain.Receipt{Pending: true, UpstreamID: "original-image-task", Body: json.RawMessage(`{}`)}, nil
		}
		polls++
		require.Equal(t, originalCredential, d.Route.Credential)
		require.Equal(t, "original-image-task", d.PollTask)
		body, err := json.Marshal(operation.GenerationResult{
			Outcome: operation.ProviderOutcomeCompleted,
			State:   operation.ProviderTaskSucceeded,
			TaskID:  d.PollTask,
			Images:  []operation.GeneratedImage{{Bytes: pngBytes(t), MediaType: "image/png"}},
		})
		require.NoError(t, err)
		return domain.Receipt{Body: body, UpstreamID: d.PollTask, PurchaseMicros: route.Purchase.Request}, nil
	})
	handlers := map[string]taskruntime.Handler{}
	runtime.RegisterWorker(handlers)
	run := func(kind, id string) {
		t.Helper()
		_, err := handlers[kind](t.Context(), taskruntime.Job{Kind: kind, BusinessID: id})
		require.NoError(t, err)
	}
	assertLive := func(id uuid.UUID) {
		t.Helper()
		current, err := legacy.Model(t.Context(), live.Name, live.Protocol)
		require.NoError(t, err)
		require.Equal(t, id, current.ID)
	}
	run(pg.OnboardingKind, route.ID.String())
	run(pg.OnboardingKind, route.ID.String())
	assertLive(live.ID)
	review, err = store.ReviewOnboarding(t.Context(), route.ID)
	require.NoError(t, err)
	require.Len(t, review.Probes, 1)
	require.Equal(t, "pending", review.Probes[0].State)
	requestID := review.Probes[0].RequestID.String()
	run("ai.poll", requestID)
	run(pg.OnboardingKind, route.ID.String())
	assertLive(live.ID)
	run("ai.deliver", requestID)
	run(pg.OnboardingKind, route.ID.String())
	assertLive(next.ID)
	require.Equal(t, 1, sends)
	require.Equal(t, 1, polls)
}

func TestOnboardingReservesSharedQuotaForLiveTraffic(t *testing.T) {
	db, legacy, model, actor := fixture(t)
	entrypointProject(t, db, actor)
	entrypointSession(t, db, actor)
	store := pg.New(db, actor, aigateway.Authority)
	route := domain.Route{ID: uuid.New(), ModelID: model.ID, Supplier: uuid.New(), Credential: uuid.New(), Group: uuid.NewString(), Protocol: model.Protocol, BaseURL: "https://8.8.8.8/v1", UpstreamModel: "supplier-model", Purchase: model.Price, Priority: 0}
	require.NoError(t, legacy.PublishRoute(t.Context(), route, []byte("test-original"), "fixture", 1, 100))
	require.NoError(t, db.Exec("UPDATE ai.route SET enabled=false WHERE model_id=? AND id<>?", model.ID, route.ID).Error)
	next := model
	next.ID = uuid.New()
	require.NoError(t, store.StageModel(t.Context(), next))
	candidate := route
	candidate.ID, candidate.ModelID = uuid.New(), next.ID
	in := application.Onboarding{Route: candidate, Scope: actor.ScopeID, Concurrency: 1, RPM: 100}
	require.NoError(t, store.PrepareOnboarding(t.Context(), in))
	review, err := store.ReviewOnboarding(t.Context(), candidate.ID)
	require.NoError(t, err)
	require.NoError(t, store.ConfirmOnboarding(t.Context(), candidate.ID, review.Plan.Credits, review.Plan.PurchaseMicros,
		func(uuid.UUID, uuid.UUID) ([]byte, string, error) { return []byte("test-candidate"), "fixture", nil }))

	upstream := providerFunc(func(_ context.Context, _ application.Dispatch, _ func(json.RawMessage) error) (domain.Receipt, error) {
		t.Error("onboarding consumed the only slot of a shared live quota")
		return domain.Receipt{}, domain.ErrUnknown
	})
	runtime := &aigateway.Runtime{DB: db, Provider: upstream}
	handlers := map[string]taskruntime.Handler{}
	runtime.RegisterWorker(handlers)
	_, err = handlers[pg.OnboardingKind](t.Context(), taskruntime.Job{Kind: pg.OnboardingKind, BusinessID: candidate.ID.String()})
	require.NoError(t, err)
	review, err = store.ReviewOnboarding(t.Context(), candidate.ID)
	require.NoError(t, err)
	require.Equal(t, "failed", review.State)
	require.Empty(t, review.Probes)
	dispatched, err := store.Admit(t.Context(), call(t, model, actor))
	require.NoError(t, err)
	require.True(t, dispatched.Send)
	require.Equal(t, route.ID, dispatched.Route.ID)
}

// The staged model must pass its real protocol probe before a project can select the release.
func TestOnboardingArkEmbeddingRequiresSettledVector(t *testing.T) {
	ctx := t.Context()
	db, _, _, actor := fixture(t)
	project := entrypointProject(t, db, actor)
	entrypointSession(t, db, actor)
	store := pg.New(db, actor, aigateway.Authority)
	price := domain.Price{Input: 1000, Currency: "CNY", FX: "1", Multiplier: "1", Discount: "1"}
	m := domain.Model{ID: uuid.New(), Name: "rag-" + uuid.NewString(), Protocol: domain.EmbeddingProtocol, Capability: "text.embed", MaxInput: 8192, MaxOutput: 1, PurchaseCap: 10000, Price: price}
	require.NoError(t, store.StageModel(ctx, m))
	engine := gin.New()
	session := engine.Group("/api", func(c *gin.Context) { c.Set("principal", actor) })
	cataloghttp.NewHandler(catalog.NewListModelsQuery(aigateway.Catalog{DB: db})).Register(session)
	projectSeesModel := func() bool {
		t.Helper()
		cursor := ""
		for {
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/models?capability=text.embed&project_id="+project.String()+"&limit=200&cursor="+cursor, nil)
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)
			var page cataloghttp.ModelPage
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
			for _, item := range page.Items {
				if item.ID == m.ID {
					require.True(t, item.Available)
					require.Equal(t, m.Name, item.Key)
					require.Equal(t, m.Capability, item.Capability)
					return true
				}
			}
			if page.NextCursor == nil {
				return false
			}
			require.NotEqual(t, cursor, *page.NextCursor)
			cursor = *page.NextCursor
		}
	}
	require.False(t, projectSeesModel())
	in := application.Onboarding{Scope: actor.ScopeID, Concurrency: 2, RPM: 1000, Route: domain.Route{ID: uuid.New(), ModelID: m.ID, Supplier: uuid.New(), Group: uuid.NewString(), Protocol: m.Protocol, BaseURL: domain.EmbeddingBaseURL, UpstreamModel: domain.EmbeddingModel, Purchase: price}}
	require.NoError(t, store.PrepareOnboarding(ctx, in))
	review, err := store.ReviewOnboarding(ctx, in.Route.ID)
	require.NoError(t, err)
	require.Len(t, review.Plan.Probes, 1)
	require.NoError(t, store.ConfirmOnboarding(ctx, in.Route.ID, review.Plan.Credits, review.Plan.PurchaseMicros, func(uuid.UUID, uuid.UUID) ([]byte, string, error) { return []byte("sealed-test-value"), "fixture", nil }))
	sends := 0
	upstream := provider.New(keyFixture{}, &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "/api/v3/embeddings/multimodal", req.URL.Path)
		sends++
		v := make([]float32, domain.EmbeddingDimensions)
		v[1] = 1
		raw, err := json.Marshal(map[string]any{"id": "probe", "model": domain.EmbeddingModel, "data": map[string]any{"embedding": v}, "usage": map[string]int{"total_tokens": 10, "prompt_tokens": 10}})
		require.NoError(t, err)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(raw))}, nil
	})}).WithLookup(func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	})
	handlers := map[string]taskruntime.Handler{}
	(&aigateway.Runtime{DB: db, Provider: upstream}).RegisterWorker(handlers)
	job := taskruntime.Job{Kind: pg.OnboardingKind, BusinessID: in.Route.ID.String()}
	for range 3 {
		_, err := handlers[pg.OnboardingKind](ctx, job)
		require.NoError(t, err)
	}
	review, err = store.ReviewOnboarding(ctx, in.Route.ID)
	require.NoError(t, err)
	require.Equal(t, "active", review.State)
	require.Equal(t, 1, sends)
	current, err := store.Model(ctx, m.Name, m.Protocol)
	require.NoError(t, err)
	require.Equal(t, m.ID, current.ID)
	require.True(t, projectSeesModel())
}

func TestRouteDeletionIsAtomicAndPreservesAdmittedAttempts(t *testing.T) {
	db, store, model, actor := fixture(t)
	ctx := t.Context()
	c := call(t, model, actor)
	dispatch, err := store.Admit(ctx, c)
	require.NoError(t, err)
	active := dispatch.Route.ID
	in := application.Onboarding{Route: domain.Route{ID: uuid.New(), ModelID: model.ID, Supplier: uuid.New(), Group: uuid.NewString(), Protocol: "chat", BaseURL: "https://8.8.8.8/v1", UpstreamModel: "pending-delete", Purchase: model.Price}, Scope: actor.ScopeID, Concurrency: 2, RPM: 10000}
	require.NoError(t, store.PrepareOnboarding(ctx, in))
	targets := []uuid.UUID{active, in.Route.ID}
	require.ErrorIs(t, store.DeleteRoutes(ctx, append(targets, uuid.New())), pg.ErrRouteNotFound)
	require.NoError(t, db.Exec("UPDATE ai.onboarding SET state='verifying' WHERE id=?", in.Route.ID).Error)
	require.ErrorIs(t, store.DeleteRoutes(ctx, targets), pg.ErrRouteVerifying)
	var enabled bool
	require.NoError(t, db.Raw("SELECT enabled FROM ai.route WHERE id=?", active).Row().Scan(&enabled))
	require.True(t, enabled)
	require.NoError(t, db.Exec("UPDATE ai.onboarding SET state='prepared' WHERE id=?", in.Route.ID).Error)
	require.ErrorIs(t, pg.New(db, actor, nil).DeleteRoutes(ctx, targets), domain.ErrForbidden)
	require.NoError(t, store.DeleteRoutes(ctx, targets))
	require.NoError(t, store.DeleteRoutes(ctx, targets))
	require.ErrorIs(t, store.PrepareOnboarding(ctx, in), domain.ErrConflict)
	retired := in
	retired.Route.ID = active
	require.ErrorIs(t, store.PrepareOnboarding(ctx, retired), domain.ErrConflict)
	var retained bool
	require.NoError(t, db.Raw("SELECT EXISTS(SELECT 1 FROM ai.attempt a JOIN ai.route r ON r.id=a.route_id WHERE a.id=? AND NOT r.enabled AND r.deleted_by=?)", dispatch.Attempt, actor.ID).Row().Scan(&retained))
	require.True(t, retained)
	_, err = store.ReviewOnboarding(ctx, in.Route.ID)
	require.ErrorIs(t, err, domain.ErrUnavailable)
	next, err := store.Admit(ctx, call(t, model, actor))
	require.NoError(t, err)
	require.NotEqual(t, active, next.Route.ID)
	require.NoError(t, store.DeleteRoutes(ctx, []uuid.UUID{next.Route.ID}))
}

func TestOnboardingConfigurationShowsActualProviderMetadata(t *testing.T) {
	_, store, model, actor := fixture(t)
	ctx := t.Context()
	in := application.Onboarding{
		Route: domain.Route{
			ID: uuid.New(), ModelID: model.ID, Supplier: uuid.New(),
			Group: uuid.NewString(), Protocol: "chat",
			BaseURL: "https://api.deepseek.com/v1", UpstreamModel: "display-only",
			Purchase: model.Price,
		},
		Scope: actor.ScopeID, Concurrency: 2, RPM: 10000,
	}
	require.NoError(t, store.PrepareOnboarding(ctx, in))
	configuration, err := store.Configuration(ctx)
	require.NoError(t, err)
	data, err := json.Marshal(configuration)
	require.NoError(t, err)
	type listing struct {
		ID          uuid.UUID `json:"id"`
		ProviderKey string    `json:"provider_key"`
		Upstream    string    `json:"upstream_model"`
	}
	var result struct {
		Onboarding []listing `json:"onboarding"`
	}
	require.NoError(t, json.Unmarshal(data, &result))
	require.True(t, slices.ContainsFunc(result.Onboarding, func(row listing) bool {
		return row.ID == in.Route.ID && row.ProviderKey == "deepseek" && row.Upstream == in.Route.UpstreamModel
	}))
}

func TestOnboardingConfigurationExcludesHistoricalLocalSpeech(t *testing.T) {
	db, store, model, actor := fixture(t)
	legacy := model
	legacy.ID, legacy.Protocol = uuid.New(), "local"
	legacy.Name = "local/whisper-" + legacy.ID.String()
	data, err := json.Marshal(legacy)
	require.NoError(t, err)
	require.NoError(t, db.Exec("INSERT INTO ai.model (id,name,document,enabled) VALUES (?,?,?::jsonb,true)", legacy.ID, legacy.Name, string(data)).Error)

	configuration, err := store.Configuration(t.Context())
	require.NoError(t, err)
	raw, err := json.Marshal(configuration["models"])
	require.NoError(t, err)
	require.NotContains(t, string(raw), legacy.Name)

	_, err = application.Prepare(legacy, actor.ID, actor.ScopeID, uuid.Nil, uuid.Nil, "retired-local", "transcription", nil)
	require.ErrorIs(t, err, domain.ErrInvalid)
}
