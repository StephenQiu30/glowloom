package aigateway_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"

	"github.com/stretchr/testify/require"

	pg "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
)

func TestProductionSessionScopeAndPlatformRevocationBeforeMoney(t *testing.T) {
	db, _, m, a := fixture(t)
	now := time.Now().UTC()
	digest := sha256.Sum256(a.SessionID[:])
	require.NoError(t, db.Exec(`UPDATE identity."user" SET email_key=?,email_verified_at=?,session_epoch=1,platform_epoch=1 WHERE id=?`, a.ID.String()+"@fixture.invalid", now, a.ID).Error)
	require.NoError(t, db.Exec(`INSERT INTO identity.user_session(id,account_id,token_hash,credential_revision,created_at,last_active_at,absolute_expires_at,persistent) VALUES(?,?,?,1,?,?,?,false)`, a.SessionID, a.ID, digest[:], now, now, now.Add(time.Hour)).Error)
	store := pg.New(db, a, aigateway.Authority)
	_, err := store.Wallet(t.Context(), a.ScopeID)
	require.NoError(t, err)
	require.NoError(t, store.Grant(t.Context(), a.ScopeID, "production-session", 10))
	_, _, _, outsider := fixture(t)
	other := outsider.ScopeID
	_, err = store.Wallet(t.Context(), other)
	require.Error(t, err)
	c := call(t, m, a)
	require.NoError(t, db.Exec(`UPDATE identity.user_session SET revoked_at=clock_timestamp() WHERE id=?`, a.SessionID).Error)
	_, err = store.Admit(t.Context(), c)
	require.ErrorIs(t, err, domain.ErrForbidden)
	require.Error(t, store.Grant(t.Context(), a.ScopeID, "revoked", 10))
	var n int64
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.request WHERE id=?", c.ID).Scan(&n).Error)
	require.Zero(t, n)
	require.NoError(t, db.Exec(`UPDATE identity.user_session SET revoked_at=NULL WHERE id=?`, a.SessionID).Error)
	require.NoError(t, db.Exec(`UPDATE identity."user" SET platform_epoch=platform_epoch+1 WHERE id=?`, a.ID).Error)
	require.Error(t, store.PublishModel(t.Context(), nativeModel("chat")))
}

func TestBusinessDatabaseRoleCanRunOneAuthorizedMoneyLifecycle(t *testing.T) {
	db, _, m, a := fixture(t)
	p := entrypointProject(t, db, a)
	entrypointSession(t, db, a)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL ROLE glowloom_app").Error; err != nil {
			return err
		}
		store := pg.New(tx, a, aigateway.Authority)
		c := call(t, m, a)
		c.Project = p
		prepared, err := application.Prepare(m, c.Actor, c.Scope, c.Project, c.Run, c.Key, c.Origin, c.Raw)
		require.NoError(t, err)
		prepared.ID = c.ID
		dispatch, err := store.Admit(t.Context(), prepared)
		require.NoError(t, err)
		require.True(t, dispatch.Send)
		usage := domain.Usage{Input: 1, Output: 1}
		cost, err := dispatch.Route.Purchase.Cost(usage)
		require.NoError(t, err)
		err = store.SaveReceipt(t.Context(), prepared.ID, dispatch.Attempt, domain.Receipt{Body: json.RawMessage(`{"choices":[]}`), Usage: usage, PurchaseMicros: cost})
		require.NoError(t, err)
		settled, err := store.Settle(t.Context(), prepared.ID)
		require.NoError(t, err)
		require.EqualValues(t, 1, settled.Charged)
		return nil
	}))
}

func TestTeamLedgerHistoryRequiresCurrentTeamAdministration(t *testing.T) {
	db, adminStore, m, a := fixture(t)
	_, _, _, member := fixture(t)
	entrypointProject(t, db, a)
	entrypointSession(t, db, a)
	entrypointProject(t, db, member)
	entrypointSession(t, db, member)
	team, sid := uuid.New(), uuid.New()
	require.NoError(t, db.Exec("INSERT INTO workspace.team(id,name,created_by) VALUES(?,'Ledger team',?)", team, a.ID).Error)
	require.NoError(t, db.Exec("INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)", sid, team, a.OrgID).Error)
	for _, entry := range []struct {
		ID   uuid.UUID
		Role string
	}{{a.ID, "team_admin"}, {member.ID, "member"}} {
		require.NoError(t, db.Exec("INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,?,'active',clock_timestamp(),'created')", team, entry.ID, entry.Role).Error)
	}
	a.ScopeID = sid
	member.ScopeID = sid
	member.OrgID = a.OrgID
	adminStore = pg.New(db, a, aigateway.Authority)
	require.NoError(t, adminStore.Grant(t.Context(), sid, "team-fund", 100))
	memberStore := pg.New(db, member, aigateway.Authority)
	provider := providerFunc(func(_ context.Context, d application.Dispatch, _ func(json.RawMessage) error) (domain.Receipt, error) {
		usage := domain.Usage{Input: 1, Output: 1}
		cost, err := d.Route.Purchase.Cost(usage)
		require.NoError(t, err)
		return domain.Receipt{Body: json.RawMessage(`{"choices":[]}`), Usage: usage, PurchaseMicros: cost}, nil
	})
	for _, entry := range []struct {
		Store *pg.Store
		Actor identity.Principal
	}{{adminStore, a}, {memberStore, member}} {
		_, err := application.New(entry.Store, provider).Execute(t.Context(), call(t, m, entry.Actor), nil)
		require.NoError(t, err)
	}
	bills, err := memberStore.Bills(t.Context(), sid)
	require.NoError(t, err)
	require.Len(t, bills, 1)
	bills, err = adminStore.Bills(t.Context(), sid)
	require.NoError(t, err)
	require.Len(t, bills, 2)
	runtime := &aigateway.Runtime{DB: db}
	wallet, err := runtime.Wallet(t.Context(), member, nil)
	require.NoError(t, err)
	require.False(t, wallet.HistoryAvailable)
	require.Empty(t, wallet.Entries)
	require.Equal(t, "98", wallet.GeneralCredits)
	adminWallet, err := runtime.Wallet(t.Context(), a, nil)
	require.NoError(t, err)
	require.True(t, adminWallet.HistoryAvailable)
	require.NotEmpty(t, adminWallet.Entries)
	require.NoError(t, db.Exec("UPDATE workspace.team_member SET role='member' WHERE team_id=? AND user_id=?", team, a.ID).Error)
	wallet, err = runtime.Wallet(t.Context(), a, nil)
	require.NoError(t, err)
	require.False(t, wallet.HistoryAvailable)
	require.Empty(t, wallet.Entries)
}
