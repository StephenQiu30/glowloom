package agentbusiness_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	agentpg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	gatewaypg "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/postgres"
	gatewayapp "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	gateway "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

const creditMicros int64 = 100000

// These fixtures execute the actual new ledger's publish, grant, admission and
// settlement contracts. Only identity access and supplier output are controlled.
type costGateway struct {
	db    *gorm.DB
	store *gatewaypg.Store
	model gateway.Model
	scope domain.Scope
	run   uuid.UUID
}

func fundedCostFixture(t *testing.T) (*gorm.DB, *agentpg.Store, domain.Admission, domain.Grant, *costGateway) {
	t.Helper()
	s, a, g := fixture(t, 100*creditMicros)
	db, err := gorm.Open(postgres.Open(os.Getenv("GLOWLOOM_TEST_AGENT_BUSINESS_DB_DSN")), &gorm.Config{})
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	require.NoError(t, db.Exec(`INSERT INTO workspace.organization(id,name) VALUES(?,'cost ledger fixture')`, a.Scope.OrgID).Error)
	require.NoError(t, db.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password) VALUES(?,?,?,'cost ledger','producer','synthetic',false)`, a.Scope.ActorID, a.Scope.OrgID, a.Scope.ActorID.String()).Error)
	require.NoError(t, db.Exec(`INSERT INTO workspace.creation_scope(id,kind,owner_user_id,physical_scope_id) VALUES(?,'personal',?,?)`, a.Scope.ScopeID, a.Scope.ActorID, a.Scope.OrgID).Error)
	require.NoError(t, db.Exec(`INSERT INTO workspace.project(id,org_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,'cost ledger','16:9','realistic')`, a.Scope.ProjectID, a.Scope.OrgID, a.Scope.ScopeID).Error)
	return db, s, a, g, newCostGateway(t, db, a.Scope, a.RootRunID)
}
func newCostGateway(t *testing.T, db *gorm.DB, scope domain.Scope, run uuid.UUID) *costGateway {
	t.Helper()
	actor := identity.Principal{ID: scope.ActorID, OrgID: scope.OrgID, ScopeID: scope.ScopeID}
	auth := func(_ context.Context, tx *gorm.DB, a identity.Principal, s, p uuid.UUID, _ bool) error {
		var n int64
		err := tx.Raw(`SELECT count(*) FROM workspace.creation_scope WHERE id=? AND owner_user_id=?`, s, a.ID).Scan(&n).Error
		if err != nil {
			return err
		}
		if n != 1 || a.ID != scope.ActorID || s != scope.ScopeID || p != uuid.Nil && p != scope.ProjectID {
			return gateway.ErrForbidden
		}
		return nil
	}
	store := gatewaypg.New(db, actor, auth).WithRunLimit(func(_ context.Context, tx *gorm.DB, id uuid.UUID) (int64, error) {
		var limit int64
		err := tx.Raw(`SELECT budget_micros FROM agent_business.run_authorization WHERE root_run_id=?`, id).Scan(&limit).Error
		return limit / creditMicros, err
	})
	price := gateway.Price{Output: 100000000000, Currency: "CNY", FX: "1", Multiplier: "1", Discount: "1"}
	model := gateway.Model{ID: uuid.New(), Name: "cost-" + uuid.NewString(), Protocol: "chat", Price: price, MaxInput: 8192, MaxOutput: 1000, PurchaseCap: 100000000}
	require.NoError(t, store.PublishModel(t.Context(), model))
	purchase := price
	purchase.Output = 1000000
	route := gateway.Route{ID: uuid.New(), ModelID: model.ID, Supplier: uuid.New(), Credential: uuid.New(), Group: uuid.NewString(), Protocol: "chat", BaseURL: "https://8.8.8.8/v1", UpstreamModel: "fixture", Purchase: purchase}
	require.NoError(t, store.PublishRoute(t.Context(), route, []byte("synthetic"), "fixture", 100, 10000))
	require.NoError(t, store.Grant(t.Context(), scope.ScopeID, "fixture/"+run.String(), 100))
	return &costGateway{db: db, store: store, model: model, scope: scope, run: run}
}
func (f *costGateway) admit(t *testing.T, id uuid.UUID, maximum int64) gatewayapp.Dispatch {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"model": f.model.Name, "messages": []map[string]string{{"role": "user", "content": "original"}}, "max_tokens": maximum})
	require.NoError(t, err)
	call, err := gatewayapp.Prepare(f.model, f.scope.ActorID, f.scope.ScopeID, f.scope.ProjectID, f.run, id.String(), "agent", raw)
	require.NoError(t, err)
	call.ID = id
	dispatch, err := f.store.Admit(t.Context(), call)
	require.NoError(t, err)
	require.True(t, dispatch.Send)
	require.Equal(t, maximum, call.Limit)
	return dispatch
}
func (f *costGateway) settle(t *testing.T, d gatewayapp.Dispatch, actual int64) gateway.Record {
	t.Helper()
	usage := gateway.Usage{Output: actual}
	purchase, err := d.Route.Purchase.Cost(usage)
	require.NoError(t, err)
	receipt := gateway.Receipt{Body: json.RawMessage(`{"choices":[{"message":{"content":"original"}}]}`), Usage: usage, PurchaseMicros: purchase}
	require.NoError(t, f.store.SaveReceipt(t.Context(), d.Call.ID, d.Attempt, receipt))
	result, err := f.store.Settle(t.Context(), d.Call.ID)
	require.NoError(t, err)
	return result
}
