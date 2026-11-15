package microservices_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	billingapp "github.com/StephenQiu30/glowloom-server/backend/execution/billing/application"
)

type projectBudgetRepository struct {
	receipt *billingapp.ProjectBudgetReceipt
	commits int
}

func (r *projectBudgetRepository) ResolveProjectBudget(_ context.Context, p billingapp.ProjectBudgetPreparation) (billingapp.ProjectBudgetReceipt, error) {
	if r.receipt == nil {
		return billingapp.ProjectBudgetReceipt{}, billingapp.ErrProjectBudgetUnknown
	}
	if r.receipt.Request != p {
		return billingapp.ProjectBudgetReceipt{}, billingapp.ErrProjectBudgetPreparation
	}
	return *r.receipt, nil
}
func (r *projectBudgetRepository) CommitProjectBudget(_ context.Context, p billingapp.ProjectBudgetPreparation, proof billingapp.ProjectPreparedProof) (billingapp.ProjectBudgetReceipt, error) {
	if proof.Request != p || proof.Phase != "prepared" {
		return billingapp.ProjectBudgetReceipt{}, billingapp.ErrProjectBudgetPreparation
	}
	r.commits++
	receipt := billingapp.NewProjectBudgetReceipt(p)
	r.receipt = &receipt
	return receipt, nil
}

type projectBudgetProof struct {
	calls    int
	err      error
	retarget bool
}

func (s *projectBudgetProof) ResolveProjectPreparation(_ context.Context, p billingapp.ProjectBudgetPreparation) (billingapp.ProjectPreparedProof, error) {
	s.calls++
	if s.err != nil {
		return billingapp.ProjectPreparedProof{}, s.err
	}
	if s.retarget {
		p.ProjectID = uuid.New()
	}
	return billingapp.ProjectPreparedProof{Request: p, Phase: "prepared"}, nil
}
func projectBudgetRequest() billingapp.ProjectBudgetPreparation {
	id := uuid.New()
	return billingapp.ProjectBudgetPreparation{PreparationID: id, ExecutionCommandID: uuid.NewSHA1(id, []byte("execution.project_budget.prepare")), SourceCommandID: uuid.New(), SourceSHA256: strings.Repeat("a", 64), InputSHA256: strings.Repeat("b", 64), ProjectID: uuid.New(), OrganizationID: uuid.New(), ScopeID: uuid.New()}
}
func TestProjectBudgetPreparationRequiresOriginalSourceAndNeverMintsCredit(t *testing.T) {
	for _, mode := range []string{"missing", "unavailable", "retargeted", "original"} {
		t.Run(mode, func(t *testing.T) {
			repo := &projectBudgetRepository{}
			proof := &projectBudgetProof{}
			if mode == "missing" {
				if _, err := billingapp.NewProjectBudgets(repo, nil); err == nil {
					t.Fatal("missing source proof accepted")
				}
				return
			}
			if mode == "unavailable" {
				proof.err = errors.New("source unavailable")
			}
			proof.retarget = mode == "retargeted"
			service, err := billingapp.NewProjectBudgets(repo, proof)
			if err != nil {
				t.Fatal(err)
			}
			req := projectBudgetRequest()
			receipt, err := service.Prepare(t.Context(), req)
			if mode != "original" {
				if err == nil || repo.commits != 0 {
					t.Fatalf("source=%s caused budget effect: %v commits=%d", mode, err, repo.commits)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if receipt.Request != req || receipt.LimitMicros != 9_007_199_254_740_991 || receipt.ReservedMicros != 0 || receipt.SpentMicros != 0 || receipt.BudgetRevision != 1 || !receipt.WalletPrepared {
				t.Fatalf("changed original uncapped/zero-wallet contract: %+v", receipt)
			}
			proof.err = errors.New("source offline after committed ACK")
			replay, err := service.Prepare(t.Context(), req)
			if err != nil || replay != receipt || proof.calls != 1 || repo.commits != 1 {
				t.Fatalf("original recovery changed responsibility: replay=%+v err=%v sourcecalls=%d commits=%d", replay, err, proof.calls, repo.commits)
			}
		})
	}
}
