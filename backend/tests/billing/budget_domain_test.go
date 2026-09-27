package billing_test

import (
	"errors"
	"math"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/billing/domain"
)

func TestBudgetChangeLimitRespectsCommittedFundsAndOverrun(t *testing.T) {
	budget := domain.Budget{
		ID: uuid.New(), ProjectID: uuid.New(), LimitMicros: 400,
		ReservedMicros: 50, SettledMicros: 300, Revision: 2,
	}
	for _, amount := range []int64{-1, 320} {
		before := budget
		_, err := budget.ChangeLimit(amount)
		if !errors.Is(err, domain.ErrLimitBelowCommitted) || budget != before {
			t.Fatalf("invalid limit %d: budget=%+v err=%v", amount, budget, err)
		}
	}
	delta, err := budget.ChangeLimit(500)
	if err != nil || delta != 100 || budget.LimitMicros != 500 || budget.Revision != 3 {
		t.Fatalf("increase: budget=%+v delta=%d err=%v", budget, delta, err)
	}
	delta, err = budget.ChangeLimit(350)
	if err != nil || delta != -150 || budget.LimitMicros != 350 || budget.Revision != 4 {
		t.Fatalf("lower to committed amount: budget=%+v delta=%d err=%v", budget, delta, err)
	}
	delta, err = budget.ChangeLimit(350)
	if err != nil || delta != 0 || budget.Revision != 4 {
		t.Fatalf("no-op: budget=%+v delta=%d err=%v", budget, delta, err)
	}

	budget.IsOverrun = true
	budget.SettledMicros = 370
	before := budget
	if _, err := budget.ChangeLimit(400); !errors.Is(err, domain.ErrLimitBelowCommitted) || budget != before {
		t.Fatalf("overrun cannot be cleared below committed: budget=%+v err=%v", budget, err)
	}
	delta, err = budget.ChangeLimit(420)
	if err != nil || delta != 70 || budget.IsOverrun || budget.Revision != 5 {
		t.Fatalf("cover overrun: budget=%+v delta=%d err=%v", budget, delta, err)
	}
}

func TestBudgetRejectsOverflowAndInvalidState(t *testing.T) {
	budget := domain.Budget{
		ID: uuid.New(), ProjectID: uuid.New(), LimitMicros: math.MaxInt64,
		ReservedMicros: math.MaxInt64, SettledMicros: 1, IsOverrun: true, Revision: 1,
	}
	if _, err := budget.ChangeLimit(math.MaxInt64); !errors.Is(err, domain.ErrInvalidBudget) {
		t.Fatalf("overflowing committed balance: %v", err)
	}
	budget.ReservedMicros, budget.SettledMicros = 0, 0
	budget.Revision = math.MaxInt32
	if _, err := budget.ChangeLimit(0); !errors.Is(err, domain.ErrInvalidBudget) {
		t.Fatalf("revision overflow: %v", err)
	}
}

func TestBudgetOverrunSameLimitIsNoOp(t *testing.T) {
	budget := domain.Budget{
		ID: uuid.New(), ProjectID: uuid.New(), LimitMicros: 100,
		ReservedMicros: 20, SettledMicros: 100, IsOverrun: true, Revision: 2,
	}
	delta, err := budget.ChangeLimit(100)
	if err != nil || delta != 0 || budget.IsOverrun != true || budget.Revision != 2 {
		t.Fatalf("same limit in overrun state = %+v delta=%d err=%v", budget, delta, err)
	}
}
