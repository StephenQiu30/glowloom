package agentbusiness_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	gatewayapp "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	gateway "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

// Bind a completed asynchronous tool receipt to the real, still-held AI request.
// No legacy operation, reservation or parallel wallet is a source of money.
func rootMediaCost(t *testing.T) (*gorm.DB, *pg.Store, domain.Admission, domain.Grant, *costGateway, gatewayapp.Dispatch, domain.CallInput) {
	t.Helper()
	db, s, a, g, ledger := fundedCostFixture(t)
	d := ledger.admit(t, uuid.New(), 80)
	in := costCall(g, 80*creditMicros)
	_, err := s.ExecuteAtomicTool(t.Context(), "opaque-confirm-tool", in, func(tx *gorm.DB, _ domain.Scope, key uuid.UUID) (domain.CallResult, error) {
		return domain.CallResult{Status: "completed", Result: json.RawMessage(`{}`)}, pg.BindOperationCostInTransaction(tx, key, d.Call.ID)
	})
	require.NoError(t, err)
	return db, s, a, g, ledger, d, in
}
func TestRootMediaCostReconcilesOriginalSettlementWithoutNotification(t *testing.T) {
	for _, tc := range []struct {
		name    string
		charge  int64
		release bool
	}{{"partial_refund", 35, false}, {"zero_usage", 0, false}, {"not_sent_release", 0, true}} {
		t.Run(tc.name, func(t *testing.T) {
			db, s, a, g, ledger, d, in := rootMediaCost(t)
			kind := "settle"
			if tc.release {
				_, err := ledger.store.ReleaseNotSent(t.Context(), d)
				require.NoError(t, err)
				kind = "release"
			} else {
				ledger.settle(t, d, tc.charge)
			}
			for range 2 {
				authority, err := s.CurrentAuthority(t.Context(), a.RootRunID)
				require.NoError(t, err)
				require.Equal(t, tc.charge*creditMicros, authority.ReservedMicros)
				fresh, err := s.ValidateGrant(t.Context(), g)
				require.NoError(t, err)
				require.Equal(t, (100-tc.charge)*creditMicros, fresh.AvailableMicros)
			}
			_, err := s.ExecuteAtomicTool(t.Context(), "opaque-confirm-tool", in, func(*gorm.DB, domain.Scope, uuid.UUID) (domain.CallResult, error) {
				t.Fatal("original confirm replay sent twice")
				return domain.CallResult{}, nil
			})
			require.NoError(t, err)
			var settlements, holds, legacy int64
			require.NoError(t, db.Raw(`SELECT count(*) FROM ai.money_event WHERE request_id=? AND kind=?`, d.Call.ID, kind).Scan(&settlements).Error)
			require.NoError(t, db.Raw(`SELECT count(*) FROM ai.hold WHERE request_id=?`, d.Call.ID).Scan(&holds).Error)
			require.NoError(t, db.Raw(`SELECT count(*) FROM billing.reservation WHERE operation_id=?`, d.Call.ID).Scan(&legacy).Error)
			require.EqualValues(t, 1, settlements)
			require.EqualValues(t, 1, holds)
			require.Zero(t, legacy)
			next := costCall(g, (100-tc.charge)*creditMicros)
			next.Call.CallID = "after-release"
			_, err = s.ExecuteCall(t.Context(), "model", "test", next, func(_ context.Context, _ domain.Scope, id uuid.UUID) (domain.CallResult, error) {
				r := ledger.settle(t, ledger.admit(t, id, 1), 1)
				return domain.CallResult{Status: "completed", Result: json.RawMessage(`{}`), CostMicros: r.Charged * creditMicros}, nil
			})
			require.NoError(t, err, "released remainder must fund a real new call")
		})
	}
}
func TestRootMediaUnknownKeepsCommitment(t *testing.T) {
	_, s, a, g, ledger, d, _ := rootMediaCost(t)
	require.NoError(t, ledger.store.Unknown(t.Context(), d.Call.ID, d.Attempt))
	authority, err := s.CurrentAuthority(t.Context(), a.RootRunID)
	require.NoError(t, err)
	require.EqualValues(t, 80*creditMicros, authority.ReservedMicros)
	next := costCall(g, 21*creditMicros)
	next.Call.CallID = "too-large"
	_, err = s.ExecuteCall(t.Context(), "model", "test", next, func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		t.Fatal("over-budget send")
		return domain.CallResult{}, nil
	})
	require.ErrorIs(t, err, domain.ErrBudget)
}
func TestRootCostSnapshotClassifiesCallsAndOriginalMoneyEvidence(t *testing.T) {
	db, s, a, g, ledger, d, in := rootMediaCost(t)
	snapshot, err := s.CostSnapshot(t.Context(), a.RootRunID)
	require.NoError(t, err)
	require.EqualValues(t, 80*creditMicros, snapshot.PendingCommitment)
	require.Zero(t, snapshot.SettledCharge)
	require.EqualValues(t, 20*creditMicros, snapshot.Remaining)
	require.Equal(t, snapshot.Remaining, snapshot.Unallocated)
	require.Zero(t, snapshot.AllocatedUnused)
	require.Equal(t, "CNY", snapshot.Currency)
	require.Equal(t, "micros", snapshot.Unit)
	require.Len(t, snapshot.Evidence, 1)
	call, err := s.ReadCall(t.Context(), a.RootRunID, g.SubmissionID, in.Call)
	require.NoError(t, err)
	require.Zero(t, call.CostMicros, "tool quote is not an actual charge")
	result := ledger.settle(t, d, 35)
	require.EqualValues(t, 35, result.Charged)
	require.EqualValues(t, 35, result.Receipt.PurchaseMicros, "original procurement remains independent from 3.5M user micros")
	snapshot, err = s.CostSnapshot(t.Context(), a.RootRunID)
	require.NoError(t, err)
	require.Zero(t, snapshot.PendingCommitment)
	require.EqualValues(t, 35*creditMicros, snapshot.SettledCharge)
	require.EqualValues(t, 65*creditMicros, snapshot.Remaining)
	e := snapshot.Evidence[0]
	require.Equal(t, &d.Call.ID, e.OperationID)
	require.Equal(t, &d.Call.ID, e.ReservationID)
	require.NotNil(t, e.SettlementID)
	require.Empty(t, e.Procurement, "root spending exposes user charges; procurement remains in immutable supplier receipt")
	var spent int64
	require.NoError(t, db.Raw(`SELECT spent_delta FROM ai.money_event WHERE id=?`, *e.SettlementID).Scan(&spent).Error)
	require.EqualValues(t, 35, spent)
	next := in
	next.Call.CallID = "second-confirm"
	_, err = s.ExecuteAtomicTool(t.Context(), "another-opaque-name", next, func(tx *gorm.DB, _ domain.Scope, id uuid.UUID) (domain.CallResult, error) {
		return domain.CallResult{Status: "completed", Result: json.RawMessage(`{}`)}, pg.BindOperationCostInTransaction(tx, id, d.Call.ID)
	})
	require.ErrorIs(t, err, domain.ErrConflict)
}
func TestRootCostSnapshotModelUnknownOverrunAndOriginalReplay(t *testing.T) {
	db, s, a, g, ledger := fundedCostFixture(t)
	in := costCall(g, 50*creditMicros)
	sends := 0
	var d gatewayapp.Dispatch
	call, err := s.ExecuteCall(t.Context(), "model", "test", in, func(_ context.Context, _ domain.Scope, id uuid.UUID) (domain.CallResult, error) {
		sends++
		d = ledger.admit(t, id, 50)
		require.NoError(t, ledger.store.Unknown(t.Context(), id, d.Attempt))
		return domain.CallResult{}, errors.New("original response lost")
	})
	require.NoError(t, err)
	snapshot, err := s.CostSnapshot(t.Context(), a.RootRunID)
	require.NoError(t, err)
	require.EqualValues(t, 50*creditMicros, snapshot.PendingCommitment)
	require.Zero(t, snapshot.SettledCharge)
	require.EqualValues(t, 50*creditMicros, snapshot.Remaining)
	_, err = s.ExecuteCall(t.Context(), "model", "test", in, func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		sends++
		return domain.CallResult{}, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, sends)
	result := ledger.settle(t, d, 125)
	require.EqualValues(t, 50, result.Charged, "frozen upper bound caps customer charge")
	require.EqualValues(t, 125, result.Receipt.PurchaseMicros, "supplier overrun evidence survives")
	actual := domain.CallResult{Status: "completed", Result: json.RawMessage(`{"original":true}`), CostMicros: result.Charged * creditMicros}
	for range 2 {
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error { _, e := pg.CompleteCall(tx, call.ReceiptID, actual); return e }))
	}
	snapshot, err = s.CostSnapshot(t.Context(), a.RootRunID)
	require.NoError(t, err)
	require.Zero(t, snapshot.PendingCommitment)
	require.EqualValues(t, 50*creditMicros, snapshot.SettledCharge)
	require.EqualValues(t, 50*creditMicros, snapshot.Remaining)
	conflict := actual
	conflict.Result = json.RawMessage(`{"original":false}`)
	require.ErrorIs(t, db.Transaction(func(tx *gorm.DB) error { _, e := pg.CompleteCall(tx, call.ReceiptID, conflict); return e }), domain.ErrConflict)
	changed := *result.Receipt
	changed.Usage.Output++
	require.ErrorIs(t, ledger.store.SaveReceipt(t.Context(), d.Call.ID, d.Attempt, changed), gateway.ErrConflict)
}
func TestRootCostIntegrityFailureBlocksChargesButPreservesFreeRecovery(t *testing.T) {
	db, s, a, g, _, d, _ := rootMediaCost(t)
	// A terminal request alone cannot prove that the original hold was closed.
	require.NoError(t, db.Exec(`UPDATE ai.request SET state='settled' WHERE id=?`, d.Call.ID).Error)
	_, err := s.CostSnapshot(t.Context(), a.RootRunID)
	require.ErrorIs(t, err, domain.ErrUnavailable)
	in := costCall(g, 0)
	in.Call.CallID = "new-paid"
	_, err = s.ExecuteCall(t.Context(), "model", "test", in, func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		t.Fatal("sent on unavailable projection")
		return domain.CallResult{}, nil
	})
	require.ErrorIs(t, err, domain.ErrUnavailable)
	in.Call.CallID = "free-original-recovery"
	_, err = s.ExecuteCall(t.Context(), "tool", "read-original", in, func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		return domain.CallResult{Status: "completed", Result: json.RawMessage(`{}`)}, nil
	})
	require.NoError(t, err)
}
func TestRootCostReadDoesNotWaitForOperationRowLock(t *testing.T) {
	db, s, a, _, _, d, _ := rootMediaCost(t)
	tx := db.Begin()
	require.NoError(t, tx.Error)
	defer tx.Rollback()
	require.NoError(t, tx.Exec(`SELECT id FROM ai.request WHERE id=? FOR UPDATE`, d.Call.ID).Error)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	snapshot, err := s.CostSnapshot(ctx, a.RootRunID)
	require.NoError(t, err)
	require.EqualValues(t, 80*creditMicros, snapshot.PendingCommitment)
}

func TestRootRejectsPaidScalarReceiptWithoutOriginalLedger(t *testing.T) {
	s, a, g := fixture(t)
	in := costCall(g, 1)
	_, err := s.ExecuteCall(t.Context(), "model", "unbacked", in, func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		return domain.CallResult{Status: "completed", Result: json.RawMessage(`{}`), CostMicros: 1}, nil
	})
	require.NoError(t, err)
	_, err = s.CostSnapshot(t.Context(), a.RootRunID)
	require.ErrorIs(t, err, domain.ErrUnavailable)
	in.Call.CallID = "cannot-spend"
	_, err = s.ExecuteCall(t.Context(), "model", "unbacked", in, func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		t.Fatal("scalar receipt granted paid authority")
		return domain.CallResult{}, nil
	})
	require.ErrorIs(t, err, domain.ErrUnavailable)
}
