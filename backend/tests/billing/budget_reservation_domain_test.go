package billing_test

import (
	"errors"
	"math"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/billing/domain"
)

func reservationBudget(limit, reserved, settled int64) domain.Budget {
	return domain.Budget{
		ID: uuid.New(), ProjectID: uuid.New(), LimitMicros: limit,
		ReservedMicros: reserved, SettledMicros: settled, Revision: 1,
	}
}

func TestBudgetReserveChecksAvailableAndRevision(t *testing.T) {
	budget := reservationBudget(100, 20, 30)
	if err := budget.Reserve(50); err != nil {
		t.Fatalf("reserve remaining balance: %v", err)
	}
	if budget.ReservedMicros != 70 || budget.SettledMicros != 30 || budget.Revision != 2 || budget.IsOverrun {
		t.Fatalf("reserved budget = %+v", budget)
	}
	before := budget
	if err := budget.Reserve(1); !errors.Is(err, domain.ErrBudgetInsufficient) || budget != before {
		t.Fatalf("reserve above available = %v, budget=%+v", err, budget)
	}
	if err := budget.Reserve(-1); !errors.Is(err, domain.ErrInvalidBudget) || budget != before {
		t.Fatalf("reserve negative = %v, budget=%+v", err, budget)
	}
	if err := budget.Reserve(0); err != nil || budget.Revision != 3 || budget.ReservedMicros != before.ReservedMicros {
		t.Fatalf("zero-cost reservation = %v, budget=%+v", err, budget)
	}
}

func TestBudgetReserveRejectsOverrunAndUnsafeStates(t *testing.T) {
	t.Run("overrun", func(t *testing.T) {
		budget := reservationBudget(100, 20, 100)
		budget.IsOverrun = true
		before := budget
		for _, amount := range []int64{0, 1} {
			if err := budget.Reserve(amount); !errors.Is(err, domain.ErrBudgetInsufficient) || budget != before {
				t.Fatalf("reserve %d while overrun = %v, budget=%+v", amount, err, budget)
			}
		}
	})
	t.Run("revision overflow", func(t *testing.T) {
		budget := reservationBudget(100, 0, 0)
		budget.Revision = math.MaxInt32
		before := budget
		if err := budget.Reserve(1); !errors.Is(err, domain.ErrInvalidBudget) || budget != before {
			t.Fatalf("reserve at max revision = %v, budget=%+v", err, budget)
		}
	})
	t.Run("invalid persisted sum", func(t *testing.T) {
		budget := reservationBudget(math.MaxInt64, math.MaxInt64, 1)
		budget.IsOverrun = true
		before := budget
		if err := budget.Reserve(1); !errors.Is(err, domain.ErrInvalidBudget) || budget != before {
			t.Fatalf("reserve on overflowing persisted sum = %v, budget=%+v", err, budget)
		}
	})
}

func TestBudgetSettleReleasesAndRecordsOverrun(t *testing.T) {
	t.Run("release unused amount", func(t *testing.T) {
		budget := reservationBudget(100, 80, 0)
		if err := budget.Settle(80, 50); err != nil {
			t.Fatalf("settle below reserve: %v", err)
		}
		if budget.ReservedMicros != 0 || budget.SettledMicros != 50 || budget.IsOverrun || budget.Revision != 2 {
			t.Fatalf("settled budget = %+v", budget)
		}
	})
	t.Run("actual cost above reserve", func(t *testing.T) {
		budget := reservationBudget(100, 80, 0)
		if err := budget.Settle(80, 130); err != nil {
			t.Fatalf("settle actual overrun: %v", err)
		}
		if budget.ReservedMicros != 0 || budget.SettledMicros != 130 || !budget.IsOverrun || budget.Revision != 2 {
			t.Fatalf("overrun budget = %+v", budget)
		}
		before := budget
		if err := budget.Reserve(1); !errors.Is(err, domain.ErrBudgetInsufficient) || budget != before {
			t.Fatalf("new reserve after overrun = %v, budget=%+v", err, budget)
		}
	})
	t.Run("zero-cost settlement", func(t *testing.T) {
		budget := reservationBudget(0, 0, 0)
		if err := budget.Settle(0, 0); err != nil || budget.Revision != 2 || budget.ReservedMicros != 0 || budget.SettledMicros != 0 {
			t.Fatalf("zero-cost settlement = %v, budget=%+v", err, budget)
		}
	})
	t.Run("overrun clears after release", func(t *testing.T) {
		budget := reservationBudget(100, 30, 100)
		budget.IsOverrun = true
		if err := budget.Settle(30, 0); err != nil || budget.IsOverrun || budget.Revision != 2 {
			t.Fatalf("release overrun reserve = %v, budget=%+v", err, budget)
		}
	})
}

func TestBudgetSettleRejectsUnsafeAmountsWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		budget   domain.Budget
		reserved int64
		charge   int64
		want     error
	}{
		{"negative reserved", reservationBudget(100, 20, 0), -1, 0, domain.ErrInvalidBudget},
		{"negative charge", reservationBudget(100, 20, 0), 1, -1, domain.ErrInvalidBudget},
		{"reservation mismatch", reservationBudget(100, 20, 0), 21, 0, domain.ErrBudgetReservationMismatch},
		{"settled overflow", reservationBudget(math.MaxInt64, 1, math.MaxInt64-1), 1, 2, domain.ErrInvalidBudget},
		{"committed overflow", reservationBudget(math.MaxInt64, 2, math.MaxInt64-2), 1, 2, domain.ErrInvalidBudget},
		{"invalid persisted state", reservationBudget(100, 120, 0), 10, 0, domain.ErrInvalidBudget},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.budget
			if err := tc.budget.Settle(tc.reserved, tc.charge); !errors.Is(err, tc.want) || tc.budget != before {
				t.Fatalf("settle unsafe state = %v, budget=%+v", err, tc.budget)
			}
		})
	}
	budget := reservationBudget(100, 20, 0)
	budget.Revision = math.MaxInt32
	before := budget
	if err := budget.Settle(20, 10); !errors.Is(err, domain.ErrInvalidBudget) || budget != before {
		t.Fatalf("settle at max revision = %v, budget=%+v", err, budget)
	}
	var nilBudget *domain.Budget
	if err := nilBudget.Reserve(0); !errors.Is(err, domain.ErrInvalidBudget) {
		t.Fatalf("reserve nil budget = %v", err)
	}
	if err := nilBudget.Settle(0, 0); !errors.Is(err, domain.ErrInvalidBudget) {
		t.Fatalf("settle nil budget = %v", err)
	}
}
