package aigateway_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	gatewayhttp "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestCreditGrantRequiresIndependentReviewAndIssuesOnce(t *testing.T) {
	db, wallet, _, operator := fixture(t, 10)
	_, _, _, reviewer := fixture(t)
	_, _, _, user := fixture(t)
	for _, actor := range []identity.Principal{operator, reviewer, user} {
		require.NoError(t, db.Exec(`UPDATE identity."user" SET email_key=?,email_verified_at=clock_timestamp() WHERE id=?`, actor.ID.String()+"@credit-fixture.invalid", actor.ID).Error)
		entrypointSession(t, db, actor)
	}
	require.NoError(t, db.Exec(`UPDATE identity."user" SET platform_role='creator' WHERE id=?`, user.ID).Error)
	user.PlatformRole = "creator"
	engine := gin.New()
	api := engine.Group("/api")
	session := api.Group("", func(c *gin.Context) {
		actors := map[string]identity.Principal{"operator": operator, "reviewer": reviewer, "creator": user}
		c.Set("principal", actors[c.GetHeader("X-Test-Actor")])
	})
	gatewayhttp.New(db, aigateway.Authority, nil, nil).Register(api, session)

	post := func(actor, route string, body any) *httptest.ResponseRecorder {
		payload, err := json.Marshal(body)
		if err != nil {
			panic(err)
		}
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/ai/admin/credits"+route, bytes.NewReader(payload))
		req.Header.Set("X-Test-Actor", actor)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}
	in := domain.CreditGrantInput{ScopeID: operator.ScopeID, Key: "approval-" + uuid.NewString(), Credits: 25, Reason: "验收赠送"}
	require.Equal(t, 403, post("creator", "", in).Code)
	require.Equal(t, 400, post("operator", "", map[string]any{"scope_id": in.ScopeID, "key": in.Key, "credits": 2.5, "reason": in.Reason}).Code)
	require.Equal(t, 400, post("operator", "", map[string]any{"scope_id": in.ScopeID, "key": in.Key, "credits": 25, "reason": in.Reason, "expires_at": "2027-01-01"}).Code)
	first := post("operator", "", in)
	require.Equal(t, 201, first.Code, first.Body.String())
	var order domain.CreditGrantOrder
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &order))
	require.Equal(t, "pending", order.State)
	require.Equal(t, first.Body.String(), post("operator", "", in).Body.String())
	changed := in
	changed.ScopeID = reviewer.ScopeID
	require.Equal(t, 409, post("operator", "", changed).Code)
	changed = in
	changed.Reason = "不同原因"
	require.Equal(t, 409, post("operator", "", changed).Code)

	balance, err := wallet.Wallet(t.Context(), operator.ScopeID)
	require.NoError(t, err)
	require.EqualValues(t, 10, balance["available"])
	route := "/" + order.ID.String() + "/review"
	approve := map[string]bool{"approve": true}
	require.Equal(t, 403, post("operator", route, approve).Code)
	require.Equal(t, 403, post("creator", route, approve).Code)

	results := make([]*httptest.ResponseRecorder, 4)
	var group sync.WaitGroup
	for i := range results {
		group.Go(func() { results[i] = post("reviewer", route, approve) })
	}
	group.Wait()
	for _, result := range results {
		require.Equal(t, 200, result.Code, result.Body.String())
		require.Equal(t, results[0].Body.String(), result.Body.String())
	}
	var issued domain.CreditGrantOrder
	require.NoError(t, json.Unmarshal(results[0].Body.Bytes(), &issued))
	require.Equal(t, "issued", issued.State)
	require.NotNil(t, issued.EventID)
	require.Equal(t, 409, post("reviewer", route, map[string]bool{"approve": false}).Code)
	balance, err = wallet.Wallet(t.Context(), operator.ScopeID)
	require.NoError(t, err)
	require.EqualValues(t, 35, balance["available"])
	var count int
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.money_event WHERE id=? AND scope_id=? AND available_delta=25", issued.EventID, operator.ScopeID).Scan(&count).Error)
	require.Equal(t, 1, count)
	projected, err := (&aigateway.Runtime{DB: db}).Wallet(t.Context(), operator, nil, 10)
	require.NoError(t, err)
	require.Equal(t, "35", projected.GeneralCredits)
	require.Equal(t, *issued.EventID, projected.Entries[0].ID)
	require.Equal(t, "promotional", projected.Entries[0].Source)

	rejectedInput := in
	rejectedInput.Key += "-reject"
	rejected := post("operator", "", rejectedInput)
	require.Equal(t, 201, rejected.Code, rejected.Body.String())
	require.NoError(t, json.Unmarshal(rejected.Body.Bytes(), &order))
	rejection := post("reviewer", "/"+order.ID.String()+"/review", map[string]bool{"approve": false})
	require.Equal(t, 200, rejection.Code, rejection.Body.String())
	var decision domain.CreditGrantOrder
	require.NoError(t, json.Unmarshal(rejection.Body.Bytes(), &decision))
	require.Equal(t, "rejected", decision.State)
	require.Nil(t, decision.EventID)

	overflowInput := in
	overflowInput.Key += "-rollback"
	overflow := post("operator", "", overflowInput)
	require.Equal(t, 201, overflow.Code, overflow.Body.String())
	require.NoError(t, json.Unmarshal(overflow.Body.Bytes(), &order))
	require.NoError(t, db.Exec("UPDATE ai.account SET available=92233720368547 WHERE scope_id=?", operator.ScopeID).Error)
	require.Equal(t, 409, post("reviewer", "/"+order.ID.String()+"/review", approve).Code)
	require.Equal(t, overflow.Body.String(), post("operator", "", overflowInput).Body.String())
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.money_event WHERE event_key=?", "credit-order/"+order.ID.String()).Scan(&count).Error)
	require.Zero(t, count)
}
