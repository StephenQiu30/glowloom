package billing_test

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	billingapp "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	billingdomain "github.com/StephenQiu30/glowloom-server/backend/internal/billing/domain"
)

func TestWalletReserveCapAndReplay(t *testing.T) {
	_, database := billingDB(t)
	actor, project := billingProject(t, database)
	seedReserveBudget(t, database, project, 1_000_000)
	scope := bindWallet(t, database, project)
	now := time.Now().UTC().Truncate(time.Microsecond)
	source := uuid.New()
	first := issueWallet(t, database, scope, source, 100_000, now.Add(time.Hour), nil)
	require.Equal(t, first, issueWallet(t, database, scope, source, 100_000, now.Add(time.Hour), nil))
	second := issueWallet(t, database, scope, uuid.New(), 100_000, now.Add(2*time.Hour), nil)
	store := billingStore(database)
	op := seedReserveOperation(t, database, project, 120_000)
	in := billingapp.ReserveInput{Actor: actor, ActorID: actor.ID, ProjectID: project, OperationID: op, AmountMicros: 120_000, CommercialPriceReady: true, OccurredAt: now}
	require.NoError(t, database.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, tx.Exec(`SET LOCAL ROLE glowloom_app`).Error)
		out, err := store.ReserveInTransaction(t.Context(), tx, in)
		if err != nil {
			return err
		}
		return attachReserveOperation(tx, op, project, out.ReservationID, actor.ID)
	}))
	require.Equal(t, int64(0), walletAvailable(t, database, first))
	require.Equal(t, int64(80_000), walletAvailable(t, database, second))

	settle := billingapp.SettleInput{ProjectID: project, OperationID: op, ActualCostMicros: 150_000, Capability: "image.generate", OccurredAt: now}
	for _, replay := range []bool{false, true} {
		require.NoError(t, database.Transaction(func(tx *gorm.DB) error {
			require.NoError(t, tx.Exec(`SET LOCAL ROLE glowloom_app`).Error)
			out, err := store.SettleInTransaction(t.Context(), tx, settle)
			require.NoError(t, err)
			require.Equal(t, int64(120_000), out.ChargeMicros)
			require.Equal(t, int64(30_000), out.ProviderOverageMicros)
			require.Equal(t, replay, out.AlreadySettled)
			return nil
		}))
	}
	var redeemed int64
	require.NoError(t, database.Raw(`SELECT sum(amount_micros)::bigint FROM billing.wallet_entry
		WHERE scope_id=? AND kind='redeem'`, scope).Scan(&redeemed).Error)
	require.Equal(t, int64(120_000), redeemed)
	view, err := store.ReadWallet(t.Context(), scope, true, nil)
	require.NoError(t, err)
	require.Equal(t, "0.8", view.GeneralCredits)
	require.Equal(t, "0", view.HeldCredits)
	require.Equal(t, "1.2", view.RedeemedToday)
	require.Len(t, view.Entries, 6)
	memberView, err := store.ReadWallet(t.Context(), scope, false, nil)
	require.NoError(t, err)
	require.Empty(t, memberView.Entries)
	require.False(t, memberView.HistoryAvailable)

	var reservation uuid.UUID
	require.NoError(t, database.Raw(`SELECT id FROM billing.reservation WHERE operation_id=?`, op).Row().Scan(&reservation))
	var compensation, repeated uuid.UUID
	for _, result := range []*uuid.UUID{&compensation, &repeated} {
		require.NoError(t, database.Raw(`SELECT billing.compensate_wallet_consumption(?,?)`,
			reservation, "verified delivery failure fixture").Row().Scan(result))
	}
	require.Equal(t, compensation, repeated)
	require.Equal(t, int64(100_000), walletAvailable(t, database, first))
	require.Equal(t, int64(100_000), walletAvailable(t, database, second))
	_, gross, _, _, _, _ := settlementRows(t, database, settle)
	require.Equal(t, int64(120_000), gross)
	require.Error(t, database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
			return err
		}
		return tx.Raw(`SELECT billing.compensate_wallet_consumption(?,?)`,
			reservation, "unprivileged").Row().Scan(&compensation)
	}))
}

func TestWalletConcurrentReserveDoesNotDoubleSpendAndRollsBackBudget(t *testing.T) {
	_, database := billingDB(t)
	actor, project := billingProject(t, database)
	seedReserveBudget(t, database, project, 1_000_000)
	scope := bindWallet(t, database, project)
	now := time.Now().UTC()
	lot := issueWallet(t, database, scope, uuid.New(), 100_000, now.Add(time.Hour), nil)
	store := billingStore(database)
	secondProject := uuid.New()
	require.NoError(t, database.Exec(`INSERT INTO workspace.project(id,org_id,owner_scope_id,name,aspect_ratio,style_type)
		VALUES (?,?,?,'second','16:9','realistic')`, secondProject, actor.OrgID, actor.ScopeID).Error)
	seedReserveBudget(t, database, secondProject, 1_000_000)
	require.Equal(t, scope, bindWallet(t, database, secondProject))
	results := make(chan error, 2)
	var group sync.WaitGroup
	for _, id := range []uuid.UUID{project, secondProject} {
		op := seedReserveOperation(t, database, id, 80_000)
		group.Go(func() {
			results <- database.Transaction(func(tx *gorm.DB) error {
				_, err := store.ReserveInTransaction(t.Context(), tx, billingapp.ReserveInput{
					Actor: actor, ActorID: actor.ID, ProjectID: id, OperationID: op, AmountMicros: 80_000, CommercialPriceReady: true, OccurredAt: now,
				})
				return err
			})
		})
	}
	group.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		} else {
			require.ErrorIs(t, err, billingdomain.ErrWalletInsufficient)
		}
	}
	require.Equal(t, 1, succeeded)
	require.Equal(t, int64(20_000), walletAvailable(t, database, lot))
	var reserved int64
	require.NoError(t, database.Raw(`SELECT sum(reserved_micros)::bigint FROM billing.budget WHERE project_id IN (?,?)`,
		project, secondProject).Scan(&reserved).Error)
	require.Equal(t, int64(80_000), reserved)
}

func bindWallet(t *testing.T, db *gorm.DB, project uuid.UUID) uuid.UUID {
	t.Helper()
	var scope uuid.UUID
	require.NoError(t, db.Raw(`SELECT billing.bind_project_wallet(?)`, project).Row().Scan(&scope))
	require.NotEqual(t, uuid.Nil, scope)
	return scope
}

func issueWallet(t *testing.T, db *gorm.DB, scope, source uuid.UUID, micros int64, expires time.Time, model *string) uuid.UUID {
	t.Helper()
	var lot uuid.UUID
	require.NoError(t, db.Raw(`SELECT billing.issue_promotional_credit(?,?,?,?,?,?)`,
		scope, source, micros, expires, model, "isolated verification").Row().Scan(&lot))
	require.NotEqual(t, uuid.Nil, lot)
	return lot
}

func walletAvailable(t *testing.T, db *gorm.DB, lot uuid.UUID) int64 {
	t.Helper()
	var amount int64
	require.NoError(t, db.Raw(`SELECT available_micros FROM billing.wallet_lot WHERE id=?`, lot).Scan(&amount).Error)
	return amount
}

func TestWalletExpiredReleaseKeepsOriginalGraceAndScope(t *testing.T) {
	_, db := billingDB(t)
	actor, project := billingProject(t, db)
	seedReserveBudget(t, db, project, 300_000)
	scope := bindWallet(t, db, project)
	now := time.Now().UTC().Truncate(time.Microsecond)
	expires := now.Add(time.Hour)
	model := "isolated.image"
	root := issueWallet(t, db, scope, uuid.New(), 100_000, expires, &model)
	store := billingStore(db)
	op := seedReserveOperation(t, db, project, 100_000)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, tx.Exec(`SET LOCAL ROLE glowloom_app`).Error)
		_, err := store.ReserveInTransaction(t.Context(), tx, billingapp.ReserveInput{
			Actor: actor, ActorID: actor.ID, ProjectID: project, OperationID: op, AmountMicros: 100_000,
			CommercialPriceReady: true, OccurredAt: now, ModelKey: &model,
		})
		return err
	}))
	held, err := store.ReadWallet(t.Context(), scope, true, nil)
	require.NoError(t, err)
	require.Equal(t, "1", held.HeldCredits)

	at := expires.Add(time.Hour)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, tx.Exec(`SET LOCAL ROLE glowloom_app`).Error)
		_, err := store.SettleInTransaction(t.Context(), tx, billingapp.SettleInput{
			ProjectID: project, OperationID: op, Capability: "image.generate", OccurredAt: at,
		})
		return err
	}))
	var returned struct {
		AvailableMicros int64
		ModelKey        string
		ExpiresAt       time.Time
	}
	require.NoError(t, db.Raw(`SELECT available_micros,model_key,expires_at FROM billing.wallet_lot WHERE root_lot_id=?`, root).Scan(&returned).Error)
	require.Equal(t, int64(100_000), returned.AvailableMicros)
	require.Equal(t, model, returned.ModelKey)
	require.True(t, returned.ExpiresAt.Equal(at.Add(7*24*time.Hour)))
	require.Equal(t, int64(0), walletAvailable(t, db, root))
}

func TestWalletBoundProjectRejectsOlderUnfundedWriter(t *testing.T) {
	_, db := billingDB(t)
	_, project := billingProject(t, db)
	seedReserveBudget(t, db, project, 300_000)
	bindWallet(t, db, project)
	id := seedReserveOperation(t, db, project, 100_000)
	err := db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(`INSERT INTO billing.reservation(id,project_id,operation_id,amount_micros,status)
			VALUES (?,?,?,100000,'held')`, uuid.New(), project, id).Error
	})
	require.ErrorContains(t, err, "wallet-backed reservation requires complete funding")
}
