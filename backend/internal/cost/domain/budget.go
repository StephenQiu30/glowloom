package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

type BudgetPolicy struct {
	ID, WorkspaceID, ProjectID string
	LimitAmount                decimal.Decimal
	Currency                   string
	Revision                   int64
	ContentHash                string
	CreatedBy, UpdatedBy       string
	CreatedAt, UpdatedAt       time.Time
}

func SameBudgetState(left, right BudgetPolicy) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.LimitAmount.Equal(right.LimitAmount) && left.Currency == right.Currency &&
		left.Revision == right.Revision && left.ContentHash == right.ContentHash
}
