package aigateway_test

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

func database(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_AI_REBUILD_DSN")
	if dsn == "" {
		t.Skip("requires isolated rebuilt AI database")
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	return conn.DB
}
func fixture(t *testing.T, initial ...int64) (*gorm.DB, *pg.Store, domain.Model, identity.Principal) {
	t.Helper()
	db := database(t)
	actor := identity.Principal{SessionID: uuid.New(), CredentialEpoch: 1, PlatformEpoch: 1, ID: uuid.New(), ScopeID: uuid.New(), OrgID: uuid.New(), PlatformRole: "platform_admin"}
	require.NoError(t, db.Exec(`INSERT INTO workspace.organization(id,name) VALUES(?,'AI isolated test')`, actor.OrgID).Error)
	require.NoError(t, db.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password,platform_role) VALUES(?,?,?,'AI Test','producer','synthetic',false,'platform_admin')`, actor.ID, actor.OrgID, "ai-"+actor.ID.String()).Error)
	require.NoError(t, db.Exec(`INSERT INTO workspace.creation_scope(id,kind,owner_user_id,physical_scope_id) VALUES(?,'personal',?,?)`, actor.ScopeID, actor.ID, actor.OrgID).Error)
	// This synthetic authority checks actual persisted ownership on each transaction;
	// production injects Identity exact-session and Creation membership checks.
	auth := func(_ context.Context, tx *gorm.DB, a identity.Principal, scope, _ uuid.UUID, admin bool) error {
		var n int64
		if err := tx.Raw(`SELECT count(*) FROM workspace.creation_scope s JOIN identity."user" u ON u.id=s.owner_user_id WHERE s.id=? AND u.id=? AND u.status='active' AND NOT u.is_delete AND (NOT ? OR u.platform_role='platform_admin')`, scope, a.ID, admin).Scan(&n).Error; err != nil {
			return err
		}
		if n != 1 {
			return domain.ErrForbidden
		}
		return nil
	}
	store := pg.New(db, actor, auth)
	price := domain.Price{Input: 1000000, CacheRead: 100000, CacheWrite: 2000000, Output: 2000000, Currency: "CNY", FX: "1", Multiplier: "1", Discount: "1"}
	model := domain.Model{ID: uuid.New(), Name: "model-" + uuid.NewString(), Protocol: "chat", Price: price, MaxInput: 8192, MaxOutput: 1000, PurchaseCap: 100000, Tools: true, Stream: true}
	require.NoError(t, store.PublishModel(t.Context(), model))
	for i := 0; i < 2; i++ {
		route := domain.Route{ID: uuid.New(), ModelID: model.ID, Supplier: uuid.New(), Credential: uuid.New(), Group: uuid.NewString(), Protocol: "chat", BaseURL: "https://8.8.8.8/v1", UpstreamModel: "supplier-model", Purchase: price, Priority: i}
		require.NoError(t, store.PublishRoute(t.Context(), route, []byte("synthetic"), "fixture", 2, 10000))
	}
	credits := int64(100)
	if len(initial) > 0 {
		credits = initial[0]
	}
	require.NoError(t, store.Grant(t.Context(), actor.ScopeID, "initial", credits))
	return db, store, model, actor
}
func call(t *testing.T, m domain.Model, a identity.Principal) domain.Call {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"model": m.Name, "max_tokens": 100, "messages": []map[string]string{{"role": "user", "content": "fixture"}}})
	c, err := app.Prepare(m, a.ID, a.ScopeID, uuid.Nil, uuid.Nil, uuid.NewString(), "test", raw)
	require.NoError(t, err)
	return c
}
func TestRebuiltFundsSingleHoldReplayAndProjection(t *testing.T) {
	db, store, model, actor := fixture(t)
	c := call(t, model, actor)
	d, err := store.Admit(t.Context(), c)
	require.NoError(t, err)
	require.True(t, d.Send)
	replay, err := store.Admit(t.Context(), c)
	require.NoError(t, err)
	require.False(t, replay.Send)
	require.Equal(t, c.ID, replay.Record.ID)
	u := domain.Usage{Input: 100, Output: 20, CacheRead: 20}
	cost, err := d.Route.Purchase.Cost(u)
	require.NoError(t, err)
	receipt := domain.Receipt{Body: json.RawMessage(`{"choices":[]}`), Usage: u, PurchaseMicros: cost}
	require.NoError(t, store.SaveReceipt(t.Context(), c.ID, d.Attempt, receipt))
	require.NoError(t, store.SaveReceipt(t.Context(), c.ID, d.Attempt, receipt))
	settled, err := store.Settle(t.Context(), c.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, settled.Charged)
	require.Equal(t, c.Limit-1, settled.Released)
	again, err := store.Settle(t.Context(), c.ID)
	require.NoError(t, err)
	require.Equal(t, settled, again)
	var actual struct{ Available, Held, EventsAvailable, EventsHeld int64 }
	require.NoError(t, db.Raw(`SELECT a.available,a.held,(SELECT sum(available_delta) FROM ai.money_event WHERE scope_id=a.scope_id) AS events_available,(SELECT sum(held_delta) FROM ai.money_event WHERE scope_id=a.scope_id) AS events_held FROM ai.account a WHERE scope_id=?`, actor.ScopeID).Scan(&actual).Error)
	require.EqualValues(t, 99, actual.Available)
	require.Zero(t, actual.Held)
	require.Equal(t, actual.Available, actual.EventsAvailable)
	require.Equal(t, actual.Held, actual.EventsHeld)
	bills, err := store.Bills(t.Context(), actor.ScopeID)
	require.NoError(t, err)
	require.Len(t, bills, 1)
	require.Equal(t, c.ID, bills[0].ID)
	receipt.Usage.Output++
	require.ErrorIs(t, store.SaveReceipt(t.Context(), c.ID, d.Attempt, receipt), domain.ErrConflict)
	require.Error(t, db.Exec("UPDATE ai.money_event SET spent_delta=0 WHERE request_id=?", c.ID).Error)
}
func TestRebuiltConcurrentFundingAndUnknownNeverReopensSend(t *testing.T) {
	db, store, m, a := fixture(t, 1)
	var calls []domain.Call
	for i := 0; i < 20; i++ {
		c := call(t, m, a)
		calls = append(calls, c)
	}
	var wg sync.WaitGroup
	var lock sync.Mutex
	success := 0
	var admitted app.Dispatch
	for _, c := range calls {
		wg.Add(1)
		go func(c domain.Call) {
			defer wg.Done()
			d, err := store.Admit(context.Background(), c)
			lock.Lock()
			defer lock.Unlock()
			if err == nil {
				success++
				admitted = d
			} else {
				require.ErrorIs(t, err, domain.ErrFunds)
			}
		}(c)
	}
	wg.Wait()
	require.Equal(t, 1, success)
	require.NoError(t, store.Unknown(t.Context(), admitted.Call.ID, admitted.Attempt))
	replay, err := store.Admit(t.Context(), admitted.Call)
	require.NoError(t, err)
	require.False(t, replay.Send)
	require.Equal(t, "unknown", replay.Record.State)
	var held int64
	require.NoError(t, db.Raw("SELECT held FROM ai.account WHERE scope_id=?", a.ScopeID).Scan(&held).Error)
	require.EqualValues(t, 1, held)
}
func TestRebuiltBackupHasIndependentAttemptAndKeepsFrozenSale(t *testing.T) {
	db, store, m, a := fixture(t)
	c := call(t, m, a)
	d, err := store.Admit(t.Context(), c)
	require.NoError(t, err)
	next, err := store.Backup(t.Context(), d)
	require.NoError(t, err)
	require.True(t, next.Send)
	require.NotEqual(t, d.Attempt, next.Attempt)
	require.NotEqual(t, d.Route.ID, next.Route.ID)
	require.Equal(t, d.Call.Model.Price, next.Call.Model.Price)
	_, err = store.Backup(t.Context(), d)
	require.Error(t, err)
	var n int64
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.attempt WHERE request_id=?", c.ID).Scan(&n).Error)
	require.EqualValues(t, 2, n)
}
func TestRebuiltSettlementFailureRecoversOriginalReceipt(t *testing.T) {
	db, store, m, a := fixture(t)
	c := call(t, m, a)
	d, err := store.Admit(t.Context(), c)
	require.NoError(t, err)
	u := domain.Usage{Input: 100, Output: 20}
	cost, _ := d.Route.Purchase.Cost(u)
	require.NoError(t, store.SaveReceipt(t.Context(), c.ID, d.Attempt, domain.Receipt{Body: json.RawMessage(`{}`), Usage: u, PurchaseMicros: cost}))
	// Corrupt only the disposable projection after saving supplier evidence;
	// settlement must stop instead of fabricating a release or a second call.
	require.NoError(t, db.Exec("UPDATE ai.account SET held=0 WHERE scope_id=?", a.ScopeID).Error)
	_, err = store.Settle(t.Context(), c.ID)
	require.ErrorIs(t, err, domain.ErrConflict)
	require.NoError(t, db.Exec("UPDATE ai.account SET held=? WHERE scope_id=?", c.Limit, a.ScopeID).Error)
	result, err := store.Settle(t.Context(), c.ID)
	require.NoError(t, err)
	require.Equal(t, "settled", result.State)
	var n int64
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.attempt WHERE request_id=?", c.ID).Scan(&n).Error)
	require.EqualValues(t, 1, n)
}
