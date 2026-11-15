package microservices_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	billingpg "github.com/StephenQiu30/glowloom-server/backend/execution/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/glowloom-server/backend/execution/billing/application"
	billingdomain "github.com/StephenQiu30/glowloom-server/backend/execution/billing/domain"
)

type budgetPreparedFixture func(context.Context, billingapp.ProjectBudgetPreparation) (billingapp.ProjectPreparedProof, error)

func (f budgetPreparedFixture) ResolveProjectPreparation(ctx context.Context, p billingapp.ProjectBudgetPreparation) (billingapp.ProjectPreparedProof, error) {
	return f(ctx, p)
}

func budgetProjectFixture(t *testing.T) (*frozenFixture, billingapp.ProjectBudgetPreparation) {
	t.Helper()
	f := newFrozenFixture(t)
	p := projectBudgetRequest()
	p.ProjectID = f.request.ProjectID
	p.ScopeID = f.request.ScopeID
	p.SourceCommandID = f.request.SourceCommand.ID
	p.SourceSHA256 = f.request.SourceCommand.SHA256
	var project struct{ OrganizationID uuid.UUID }
	if err := f.creation.Raw(`SELECT org_id AS organization_id FROM workspace.project WHERE id=?`, p.ProjectID).Scan(&project).Error; err != nil {
		t.Fatal(err)
	}
	p.OrganizationID = project.OrganizationID
	return f, p
}

// These tests accept an explicit private source fixture. They validate the actual
// restricted Execution transaction, not an unimplemented Creation public project API.
func TestProjectBudgetPostgresOriginalACKKeepsExistingWalletAndUncappedOpening(t *testing.T) {
	f, p := budgetProjectFixture(t)
	lot := uuid.New()
	if err := f.admin.Exec(`INSERT INTO billing.wallet_account(scope_id,version) VALUES(?,7)`, p.ScopeID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.admin.Exec(`INSERT INTO billing.wallet_lot(id,scope_id,source_kind,source_id,issued_micros,available_micros,reason) VALUES(?,?,'paid',?,5000,3210,'synthetic pre-existing wallet')`, lot, p.ScopeID, uuid.New()).Error; err != nil {
		t.Fatal(err)
	}
	pool, err := f.execution.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	calls := 0
	sourceOffline := false
	source := budgetPreparedFixture(func(ctx context.Context, request billingapp.ProjectBudgetPreparation) (billingapp.ProjectPreparedProof, error) {
		calls++
		if sourceOffline {
			return billingapp.ProjectPreparedProof{}, errors.New("synthetic source offline")
		}
		// With one Execution connection, this would time out if preparation held its
		// SQL transaction open while asking Creation for the original source proof.
		if err := f.execution.WithContext(ctx).Exec(`SELECT 1`).Error; err != nil {
			return billingapp.ProjectPreparedProof{}, err
		}
		var project struct{ OrganizationID, ScopeID uuid.UUID }
		if err := f.creation.WithContext(ctx).Raw(`SELECT org_id AS organization_id,owner_scope_id AS scope_id FROM workspace.project WHERE id=?`, request.ProjectID).Scan(&project).Error; err != nil {
			return billingapp.ProjectPreparedProof{}, err
		}
		if project.OrganizationID != request.OrganizationID || project.ScopeID != request.ScopeID {
			return billingapp.ProjectPreparedProof{}, billingapp.ErrProjectBudgetPreparation
		}
		return billingapp.ProjectPreparedProof{Request: request, Phase: "prepared"}, nil
	})
	service, err := billingapp.NewProjectBudgets(billingpg.NewProjectBudgetStore(f.execution), source)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	receipt, err := service.Prepare(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if receipt != billingapp.NewProjectBudgetReceipt(p) {
		t.Fatalf("changed initial receipt: %+v", receipt)
	}
	var facts struct {
		LimitMicros, ReservedMicros, SpentMicros, WalletVersion, LotAvailable int64
		MappedScope                                                           uuid.UUID
		Lots, Reservations, WalletEntries                                     int
	}
	query := `SELECT b.limit_micros,b.reserved_micros,b.settled_micros AS spent_micros,a.version AS wallet_version,w.scope_id AS mapped_scope,(SELECT available_micros FROM billing.wallet_lot WHERE id=?) AS lot_available,(SELECT count(*) FROM billing.wallet_lot WHERE scope_id=?) AS lots,(SELECT count(*) FROM billing.reservation WHERE project_id=b.project_id) AS reservations,(SELECT count(*) FROM billing.wallet_entry WHERE scope_id=?) AS wallet_entries FROM billing.budget b JOIN billing.project_wallet w ON w.project_id=b.project_id JOIN billing.wallet_account a ON a.scope_id=w.scope_id WHERE b.id=?`
	if err := f.execution.Raw(query, lot, p.ScopeID, p.ScopeID, receipt.BudgetID).Scan(&facts).Error; err != nil {
		t.Fatal(err)
	}
	if facts.LimitMicros != 9_007_199_254_740_991 || facts.ReservedMicros != 0 || facts.SpentMicros != 0 || facts.WalletVersion != 7 || facts.LotAvailable != 3210 || facts.MappedScope != p.ScopeID || facts.Lots != 1 || facts.Reservations != 0 || facts.WalletEntries != 0 {
		t.Fatalf("preparation changed credit/wallet policy: %+v", facts)
	}
	// Quote funds simulation uses only Billing-owned payer metadata, while its
	// caller remains responsible for current Creation scope/project authorization.
	demand := []billingapp.WalletDemand{{Micros: 1234, ModelKey: "synthetic-model", CommercialPriceReady: true}}
	cash := billingpg.NewStore(f.execution)
	if err := cash.CheckWalletQuoteInTransaction(f.execution, p.ProjectID, demand, p.ScopeID); err != nil {
		t.Fatal("owned source payer estimate", err)
	}
	if err := cash.CheckWalletQuoteInTransaction(f.execution, p.ProjectID, demand); !errors.Is(err, billingdomain.ErrWalletPayerChanged) {
		t.Fatalf("missing source scope inferred permission: %v", err)
	}
	if err := cash.CheckWalletQuoteInTransaction(f.execution, p.ProjectID, demand, uuid.New()); !errors.Is(err, billingdomain.ErrWalletPayerChanged) {
		t.Fatalf("retargeted source payer accepted: %v", err)
	}
	demand[0].Micros = 4000
	if err := cash.CheckWalletQuoteInTransaction(f.execution, p.ProjectID, demand, p.ScopeID); !errors.Is(err, billingdomain.ErrWalletInsufficient) {
		t.Fatalf("source payer bypassed real lot amount: %v", err)
	}
	if err := f.execution.Exec(`UPDATE billing.budget SET limit_micros=5000,revision=revision+1 WHERE id=?`, receipt.BudgetID).Error; err != nil {
		t.Fatal(err)
	}
	sourceOffline = true
	restarted, err := billingapp.NewProjectBudgets(billingpg.NewProjectBudgetStore(f.execution), source)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := restarted.Prepare(t.Context(), p)
	if err != nil || replay != receipt || calls != 1 {
		t.Fatalf("original ACK replay=%+v err=%v sourcecalls=%d", replay, err, calls)
	}
	var current int64
	if err := f.execution.Raw(`SELECT limit_micros FROM billing.budget WHERE id=?`, receipt.BudgetID).Scan(&current).Error; err != nil || current != 5000 {
		t.Fatalf("replay reset current budget: %d err=%v", current, err)
	}
	altered := p
	altered.OrganizationID = uuid.New()
	if _, err := restarted.Resolve(t.Context(), altered); !errors.Is(err, billingapp.ErrProjectBudgetPreparation) {
		t.Fatalf("retargeted source recovered original ACK: %v", err)
	}
}

func TestProjectBudgetPostgresResourceFailureRollsBackWalletAndBudget(t *testing.T) {
	f, p := budgetProjectFixture(t)
	p.ProjectID = uuid.New()
	p.ScopeID = uuid.New()
	repo := billingpg.NewProjectBudgetStore(f.execution)
	// Fault-injected source proof deliberately references a missing actual project.
	// The retained project_wallet FK must roll back the whole local participant.
	if _, err := repo.CommitProjectBudget(t.Context(), p, billingapp.ProjectPreparedProof{Request: p, Phase: "prepared"}); err == nil {
		t.Fatal("missing actual project accepted")
	} else {
		var state interface{ SQLState() string }
		if !errors.As(err, &state) || state.SQLState() != "23503" {
			t.Fatalf("expected real project FK rollback, got %v", err)
		}
	}
	var counts struct{ Wallets, Budgets, Receipts int }
	if err := f.execution.Raw(`SELECT (SELECT count(*) FROM billing.wallet_account WHERE scope_id=?) AS wallets,(SELECT count(*) FROM billing.budget WHERE project_id=?) AS budgets,(SELECT count(*) FROM billing.project_budget_preparation WHERE execution_command_id=?) AS receipts`, p.ScopeID, p.ProjectID, p.ExecutionCommandID).Scan(&counts).Error; err != nil {
		t.Fatal(err)
	}
	if counts.Wallets != 0 || counts.Budgets != 0 || counts.Receipts != 0 {
		t.Fatalf("partial preparation escaped rollback: %+v", counts)
	}
}

func TestProjectBudgetPostgresForeignOwnersCannotPrepareOrInspect(t *testing.T) {
	_, p := budgetProjectFixture(t)
	for _, owner := range []string{"identity", "creation", "media"} {
		t.Run(owner, func(t *testing.T) {
			db := authorityDatabase(t, owner)
			calls := 0
			service, err := billingapp.NewProjectBudgets(billingpg.NewProjectBudgetStore(db), budgetPreparedFixture(func(context.Context, billingapp.ProjectBudgetPreparation) (billingapp.ProjectPreparedProof, error) {
				calls++
				return billingapp.ProjectPreparedProof{}, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Prepare(t.Context(), p); !errors.Is(err, billingapp.ErrProjectBudgetOwner) || calls != 0 {
				t.Fatalf("foreign owner=%s effect=%v sourcecalls=%d", owner, err, calls)
			}
			for _, sql := range []string{`SELECT * FROM billing.project_budget_preparation LIMIT 0`, `UPDATE billing.project_budget_preparation SET receipt='{}' WHERE false`, `DELETE FROM billing.project_budget_preparation WHERE false`} {
				if err := db.Exec(sql).Error; err == nil {
					t.Fatal("foreign owner gained budget preparation ledger", sql)
				}
			}
		})
	}
}
