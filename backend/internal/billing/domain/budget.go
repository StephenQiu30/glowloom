// Package domain defines the project budget and immutable ledger facts.
package domain

import (
	"errors"
	"math"

	"github.com/google/uuid"
)

var (
	// ErrInvalidBudget means the budget state cannot be represented safely.
	ErrInvalidBudget = errors.New("invalid budget")
	// ErrLimitBelowCommitted means a new limit would underfund existing charges.
	ErrLimitBelowCommitted = errors.New("budget limit below committed funds")
	// ErrBudgetRevision means another transaction changed the observed budget.
	ErrBudgetRevision = errors.New("budget revision conflict")
)

// Budget is the project's spending envelope. Money is always in integer
// micros; an explicit overrun records an actual settlement above the limit.
type Budget struct {
	ID             uuid.UUID
	ProjectID      uuid.UUID
	LimitMicros    int64
	ReservedMicros int64
	SettledMicros  int64
	IsOverrun      bool
	Revision       int64
}

// Validate checks the persisted balance without performing arithmetic that
// could overflow when reservation and settlement are both large.
func (b Budget) Validate() error {
	if b.ID == uuid.Nil || b.ProjectID == uuid.Nil || b.LimitMicros < 0 ||
		b.ReservedMicros < 0 || b.SettledMicros < 0 ||
		b.Revision < 1 || b.Revision > math.MaxInt32 ||
		b.ReservedMicros > math.MaxInt64-b.SettledMicros {
		return ErrInvalidBudget
	}
	committed := b.ReservedMicros + b.SettledMicros
	if b.IsOverrun != (committed > b.LimitMicros) {
		return ErrInvalidBudget
	}
	return nil
}

// AvailableMicros may be negative after an actual settlement overrun.
func (b Budget) AvailableMicros() (int64, error) {
	if err := b.Validate(); err != nil {
		return 0, err
	}
	return b.LimitMicros - b.ReservedMicros - b.SettledMicros, nil
}

// ChangeLimit changes only the configured limit and returns the signed amount
// for one budget_change ledger entry. A no-op returns zero without a revision.
func (b *Budget) ChangeLimit(next int64) (int64, error) {
	if b == nil || b.Validate() != nil {
		return 0, ErrInvalidBudget
	}
	if next == b.LimitMicros {
		return 0, nil
	}
	if next < 0 || next < b.ReservedMicros+b.SettledMicros {
		return 0, ErrLimitBelowCommitted
	}
	if b.Revision == math.MaxInt32 {
		return 0, ErrInvalidBudget
	}
	delta := next - b.LimitMicros
	b.LimitMicros = next
	b.IsOverrun = false
	b.Revision++
	return delta, nil
}
