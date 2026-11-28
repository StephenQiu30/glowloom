package aigateway_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	gatewayhttp "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/http"
	pg "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

func TestPublishModelSaleFreezesExistingQuotesAndKeepsVerifiedSupply(t *testing.T) {
	db, store, original, actor := fixture(t, 10000)
	ctx := t.Context()
	quoted := call(t, original, actor)
	priorQuote, err := store.Quote(ctx, quoted)
	require.NoError(t, err)
	inFlight, err := store.Admit(ctx, call(t, original, actor))
	require.NoError(t, err)
	require.True(t, inFlight.Send)

	price := original.Price
	price.Multiplier = "100"
	change := pg.SaleChange{Price: price}
	engine := gin.New()
	api := engine.Group("/api")
	session := api.Group("", func(c *gin.Context) { c.Set("principal", actor) })
	gatewayhttp.New(db, fixtureAuthority, nil, nil).Register(api, session)
	saleURL := "/api/ai/admin/models/" + original.ID.String() + "/price"
	send := func(method, path string, payload any) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		req := httptest.NewRequestWithContext(ctx, method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response
	}
	usage := domain.Usage{Input: 100, Output: 20}
	preview := send(http.MethodPost, saleURL+"/preview", struct {
		pg.SaleChange
		Usage domain.Usage `json:"usage"`
	}{change, usage})
	require.Equal(t, http.StatusOK, preview.Code, preview.Body.String())
	expectedMicros, err := price.Cost(usage)
	require.NoError(t, err)
	expectedCredits, err := domain.Credits(expectedMicros)
	require.NoError(t, err)
	var result struct{ Credits int64 }
	require.NoError(t, json.Unmarshal(preview.Body.Bytes(), &result))
	require.Equal(t, expectedCredits, result.Credits)
	var versions int64
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.model_sale WHERE model_id=?", original.ID).Row().Scan(&versions))
	require.Zero(t, versions)

	response := send(http.MethodPut, saleURL, change)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var published domain.Model
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &published))
	require.Equal(t, original.ID, published.ID)
	require.EqualValues(t, 1, published.SaleVersion)
	_, err = store.ModelSale(ctx, original.ID, change, true)
	require.ErrorIs(t, err, domain.ErrConflict)

	resolved, err := store.Model(ctx, original.Name, original.Protocol)
	require.NoError(t, err)
	require.Equal(t, published, resolved)
	newQuote, err := store.Quote(ctx, call(t, resolved, actor))
	require.NoError(t, err)
	require.Greater(t, newQuote.Credits, priorQuote.Credits)
	replay, err := application.Prepare(resolved, quoted.Actor, quoted.Scope, quoted.Project, quoted.Run, quoted.Key, quoted.Origin, quoted.Raw)
	require.NoError(t, err)
	replayed, err := store.Quote(ctx, replay)
	require.NoError(t, err)
	require.Equal(t, priorQuote.Credits, replayed.Credits)
	frozen, err := store.LoadQuote(ctx, actor.ScopeID, quoted.ID)
	require.NoError(t, err)
	require.Equal(t, original.Price, frozen.Model.Price)
	fromQuote, err := store.Admit(ctx, frozen)
	require.NoError(t, err)
	require.True(t, fromQuote.Send)
	_, err = store.Admit(ctx, call(t, original, actor))
	require.ErrorIs(t, err, domain.ErrUnavailable)

	// A procurement revision affects only subsequent attempts. Neither price
	// publication creates a new route, credential, or upstream trial.
	purchase := inFlight.Route.Purchase
	purchase.Multiplier = "2"
	nextRoute, err := store.PublishRoutePurchase(ctx, inFlight.Route.ID, pg.SaleChange{Price: purchase})
	require.NoError(t, err)
	require.Equal(t, inFlight.Route.Credential, nextRoute.Credential)
	require.EqualValues(t, 1, nextRoute.PurchaseVersion)
	cost, err := inFlight.Route.Purchase.Cost(usage)
	require.NoError(t, err)
	require.NoError(t, store.SaveReceipt(ctx, inFlight.Call.ID, inFlight.Attempt, domain.Receipt{
		Usage: usage, PurchaseMicros: cost, Body: json.RawMessage(`{"choices":[]}`),
	}))
	settled, err := store.Settle(ctx, inFlight.Call.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, settled.Charged)
	ready, err := store.Admit(ctx, newQuote.Call)
	require.NoError(t, err)
	require.Equal(t, inFlight.Route.ID, ready.Route.ID)
	require.Equal(t, purchase, ready.Route.Purchase)
	require.Equal(t, price, ready.Call.Model.Price)

	require.NoError(t, db.Raw("SELECT count(*) FROM ai.route WHERE model_id=?", original.ID).Row().Scan(&versions))
	require.EqualValues(t, 2, versions)
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.onboarding WHERE model_id=?", original.ID).Row().Scan(&versions))
	require.Zero(t, versions)
}
