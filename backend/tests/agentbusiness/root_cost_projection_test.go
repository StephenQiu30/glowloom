package agentbusiness_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billing "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

// The fixture seeds an original held reservation; settlement is performed by the
// real billing adapter, without a provider request or a second wallet hold.
func rootMediaCost(t *testing.T) (*gorm.DB, *pg.Store, domain.Admission, domain.Grant, uuid.UUID, domain.CallInput) {
	t.Helper()
	s, a, g := fixture(t)
	db, err := gorm.Open(postgres.Open(os.Getenv("GLOWLOOM_TEST_AGENT_BUSINESS_DB_DSN")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	t.Cleanup(func() { _ = conn.Close() })
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspace.organization(id,name) VALUES(?,'root cost test')`, []any{a.Scope.OrgID}},
		{`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password) VALUES(?,?,?,'root cost','producer','synthetic',false)`, []any{a.Scope.ActorID, a.Scope.OrgID, a.Scope.ActorID.String()}},
	} {
		if err = db.Exec(stmt.sql, stmt.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	actor := identityfixture.Session(t, db, a.Scope.Principal())
	if err = db.Exec(`INSERT INTO workspace.project(id,org_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,'root cost','16:9','realistic')`, a.Scope.ProjectID, a.Scope.OrgID, actor.ScopeID).Error; err != nil {
		t.Fatal(err)
	}
	op, res := uuid.New(), uuid.New()
	if err = db.Exec(`INSERT INTO operation.operation(id,project_id,target_type,capability,mode,input_hash,origin,status,quote_micros) VALUES(?,?,'free','image.generate','text_to_image','cost-test','upload','confirmed',80)`, op, a.Scope.ProjectID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`INSERT INTO billing.budget(id,project_id,limit_micros,reserved_micros) VALUES(?,?,100,80)`, uuid.New(), a.Scope.ProjectID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`INSERT INTO billing.reservation(id,project_id,operation_id,amount_micros,status) VALUES(?,?,?,80,'held')`, res, a.Scope.ProjectID, op).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`UPDATE operation.operation SET reservation_id=? WHERE id=?`, res, op).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`INSERT INTO billing.ledger_entry(id,project_id,operation_id,entry_type,amount_micros) VALUES(?,?,?,'reserve',80)`, uuid.New(), a.Scope.ProjectID, op).Error; err != nil {
		t.Fatal(err)
	}
	in := costCall(g, 80)
	_, err = s.ExecuteAtomicTool(t.Context(), "opaque-confirm-tool", in, func(tx *gorm.DB, _ domain.Scope, key uuid.UUID) (domain.CallResult, error) {
		err := pg.BindOperationCostInTransaction(tx, key, op)
		return domain.CallResult{Status: "completed", Result: json.RawMessage(`{}`), CostMicros: 0}, err
	})
	if err != nil {
		t.Fatal(err)
	}
	return db, s, a, g, op, in
}

func settleRootMedia(t *testing.T, db *gorm.DB, a domain.Admission, op uuid.UUID, actual int64, status string) {
	t.Helper()
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT id FROM operation.operation WHERE id=? FOR UPDATE`, op).Error; err != nil {
			return err
		}
		out, err := billingpg.NewStore(tx).SettleInTransaction(t.Context(), tx, billing.SettleInput{ProjectID: a.Scope.ProjectID, OperationID: op, ActualCostMicros: actual, Capability: "image.generate", OccurredAt: time.Now()})
		if err != nil {
			return err
		}
		return tx.Exec(`UPDATE operation.operation SET status=?,settled_micros=? WHERE id=?`, status, out.ChargeMicros, op).Error
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRootMediaCostReconcilesOriginalSettlementWithoutNotification(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		charge       int64
	}{{"partial_refund", "completed", 35}, {"failed_refund", "failed", 0}, {"cancelled_refund", "cancelled", 0}} {
		t.Run(tc.name, func(t *testing.T) {
			db, s, a, g, op, in := rootMediaCost(t)
			settleRootMedia(t, db, a, op, tc.charge, tc.status)
			for range 2 {
				authority, err := s.CurrentAuthority(t.Context(), a.RootRunID)
				if err != nil || authority.ReservedMicros != tc.charge {
					t.Fatalf("quote did not converge to original user charge: %+v %v", authority, err)
				}
				fresh, err := s.ValidateGrant(t.Context(), g)
				if err != nil || fresh.AvailableMicros != 100-tc.charge {
					t.Fatalf("admission uses stale commitment: %+v %v", fresh, err)
				}
			}
			if _, err := s.ExecuteAtomicTool(t.Context(), "opaque-confirm-tool", in, func(*gorm.DB, domain.Scope, uuid.UUID) (domain.CallResult, error) {
				t.Fatal("original confirm replay sent twice")
				return domain.CallResult{}, nil
			}); err != nil {
				t.Fatal(err)
			}
			var settlements, holds int64
			db.Raw(`SELECT count(*) FROM billing.ledger_entry WHERE operation_id=? AND entry_type='settle'`, op).Scan(&settlements)
			db.Raw(`SELECT count(*) FROM billing.wallet_hold WHERE reservation_id=(SELECT reservation_id FROM operation.operation WHERE id=?)`, op).Scan(&holds)
			if settlements != 1 || holds != 0 {
				t.Fatalf("projection changed funds: settlements=%d wallet holds=%d", settlements, holds)
			}
			next := costCall(g, 100-tc.charge)
			next.Call.CallID = "after-release"
			if _, err := s.ExecuteCall(t.Context(), "model", "test", next, func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
				return domain.CallResult{Status: "completed", Result: json.RawMessage(`{}`)}, nil
			}); err != nil {
				t.Fatalf("released remainder unusable: %v", err)
			}
		})
	}
}

func TestRootMediaUnknownKeepsCommitment(t *testing.T) {
	db, s, a, g, op, _ := rootMediaCost(t)
	if err := db.Exec(`UPDATE operation.operation SET status='unknown' WHERE id=?`, op).Error; err != nil {
		t.Fatal(err)
	}
	authority, err := s.CurrentAuthority(t.Context(), a.RootRunID)
	if err != nil || authority.ReservedMicros != 80 {
		t.Fatal(authority, err)
	}
	next := costCall(g, 21)
	next.Call.CallID = "too-large"
	if _, err := s.ExecuteCall(t.Context(), "model", "test", next, func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		t.Fatal("over-budget send")
		return domain.CallResult{}, nil
	}); !errors.Is(err, domain.ErrBudget) {
		t.Fatal(err)
	}
}

func TestRootCostSnapshotClassifiesCallsAndOriginalMoneyEvidence(t *testing.T) {
	db, s, a, g, op, in := rootMediaCost(t)
	snapshot, err := s.CostSnapshot(t.Context(), a.RootRunID)
	if err != nil || snapshot.PendingCommitment != 80 || snapshot.SettledCharge != 0 || snapshot.Remaining != 20 || snapshot.Unallocated != 20 || snapshot.AllocatedUnused != 0 || snapshot.Currency != "CNY" || snapshot.Unit != "micros" || len(snapshot.Evidence) != 1 || len(snapshot.Evidence[0].Procurement) != 0 {
		t.Fatalf("pending snapshot: %+v %v", snapshot, err)
	}
	call, err := s.ReadCall(t.Context(), a.RootRunID, g.SubmissionID, in.Call)
	if err != nil || call.CostMicros != 0 {
		t.Fatalf("tool quote masquerades as actual charge: %+v %v", call, err)
	}
	settleRootMedia(t, db, a, op, 35, "completed")
	provider, profile, sale, tariff, providerCall := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO catalog.provider(id,key,name,adapter_key,region) VALUES(?,?,'cost test','mock','domestic')`, []any{provider, provider.String()}},
		{`INSERT INTO catalog.model_profile(id,model_key,provider_id,capability,display_name,status) VALUES(?,?,?,'image.generate','cost test','active')`, []any{profile, profile.String(), provider}},
		{`INSERT INTO catalog.price_rule_version(id,model_profile_id,version_no,unit,rule,currency,fx_rate_to_cny,effective_from) VALUES(?,?,1,'per_image','{}','USD',7,now())`, []any{tariff, profile}},
		{`INSERT INTO catalog.price_rule_version(id,model_profile_id,version_no,unit,rule,currency,effective_from) VALUES(?,?,2,'per_image',jsonb_build_object('procurement_price_version_id',?::text),'CNY',now())`, []any{sale, profile, tariff.String()}},
		{`INSERT INTO operation.provider_call(id,project_id,operation_id,attempt,action,provider_key,request_summary,response_summary,outcome,region,cost_micros,procurement_cost_micros,price_rule_version_id) VALUES(?,?,?,1,'submit','cost-test','{}','{"state":"completed"}','ok','domestic',35,9,?)`, []any{providerCall, a.Scope.ProjectID, op, sale}},
	} {
		if err = db.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err = s.CostSnapshot(t.Context(), a.RootRunID)
	if err != nil || snapshot.PendingCommitment != 0 || snapshot.SettledCharge != 35 || snapshot.Remaining != 65 || len(snapshot.Evidence) != 1 {
		t.Fatalf("settled snapshot: %+v %v", snapshot, err)
	}
	e := snapshot.Evidence[0]
	if e.OperationID == nil || *e.OperationID != op || e.ReservationID == nil || e.SettlementID == nil || len(e.Procurement) != 1 || e.Procurement[0].Currency != "CNY" || e.Procurement[0].TariffCurrency != "USD" || e.Procurement[0].AmountMicros != 9 || e.Procurement[0].PriceVersionID == nil || *e.Procurement[0].PriceVersionID != tariff {
		t.Fatalf("original money evidence lost or mixed: %+v", e)
	}
	// Another receipt cannot reparent the original operation, even in the same root.
	next := in
	next.Call.CallID = "second-confirm"
	if _, err = s.ExecuteAtomicTool(t.Context(), "another-opaque-name", next, func(tx *gorm.DB, _ domain.Scope, id uuid.UUID) (domain.CallResult, error) {
		return domain.CallResult{Status: "completed", Result: json.RawMessage(`{}`)}, pg.BindOperationCostInTransaction(tx, id, op)
	}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("operation double-accounted: %v", err)
	}
}

func TestRootCostSnapshotModelUnknownOverrunAndOriginalReplay(t *testing.T) {
	s, a, g := fixture(t)
	in := costCall(g, 50)
	sends := 0
	call, err := s.ExecuteCall(t.Context(), "model", "test", in, func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		sends++
		return domain.CallResult{}, errors.New("original response lost")
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.CostSnapshot(t.Context(), a.RootRunID)
	if err != nil || snapshot.PendingCommitment != 50 || snapshot.SettledCharge != 0 || snapshot.Remaining != 50 {
		t.Fatalf("unknown released original capacity: %+v %v", snapshot, err)
	}
	if _, err = s.ExecuteCall(t.Context(), "model", "test", in, func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		sends++
		return domain.CallResult{}, nil
	}); err != nil || sends != 1 {
		t.Fatalf("unknown resent: %d %v", sends, err)
	}
	db, err := gorm.Open(postgres.Open(os.Getenv("GLOWLOOM_TEST_AGENT_BUSINESS_DB_DSN")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	})
	actual := domain.CallResult{Status: "completed", Result: json.RawMessage(`{"original":true}`), CostMicros: 125}
	for range 2 {
		if err = db.Transaction(func(tx *gorm.DB) error { _, err := pg.CompleteCall(tx, call.ReceiptID, actual); return err }); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err = s.CostSnapshot(t.Context(), a.RootRunID)
	if err != nil || snapshot.PendingCommitment != 0 || snapshot.SettledCharge != 125 || snapshot.Remaining != 0 {
		t.Fatalf("overrun truncated or double-counted: %+v %v", snapshot, err)
	}
	conflict := actual
	conflict.Result = json.RawMessage(`{"original":false}`)
	if err = db.Transaction(func(tx *gorm.DB) error { _, err := pg.CompleteCall(tx, call.ReceiptID, conflict); return err }); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("changed original receipt accepted: %v", err)
	}
}

func TestRootCostIntegrityFailureBlocksChargesButPreservesFreeRecovery(t *testing.T) {
	db, s, a, g, op, _ := rootMediaCost(t)
	// A terminal operation alone cannot prove money released. Original held
	// reservation and ledger must close together before any capacity returns.
	if err := db.Exec(`UPDATE operation.operation SET status='failed',settled_micros=0 WHERE id=?`, op).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.CostSnapshot(t.Context(), a.RootRunID); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("incoherent funds published: %v", err)
	}
	in := costCall(g, 0)
	in.Call.CallID = "new-paid"
	if _, err := s.ExecuteCall(t.Context(), "model", "test", in, func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		t.Fatal("sent on unavailable projection")
		return domain.CallResult{}, nil
	}); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
	in.Call.CallID = "free-original-recovery"
	if _, err := s.ExecuteCall(t.Context(), "tool", "read-original", in, func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		return domain.CallResult{Status: "completed", Result: json.RawMessage(`{}`)}, nil
	}); err != nil {
		t.Fatalf("original recovery unavailable: %v", err)
	}
}

func TestRootCostReadDoesNotWaitForOperationRowLock(t *testing.T) {
	db, s, a, _, op, _ := rootMediaCost(t)
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	if err := tx.Exec(`SELECT id FROM operation.operation WHERE id=? FOR UPDATE`, op).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	snapshot, err := s.CostSnapshot(ctx, a.RootRunID)
	if err != nil || snapshot.PendingCommitment != 80 {
		t.Fatalf("root read reversed operation lock order: %+v %v", snapshot, err)
	}
}
