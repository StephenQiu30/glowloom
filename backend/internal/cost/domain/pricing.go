package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

const (
	MetricGenerationImage  = "generation.image"
	SourceGenerationIntent = "generation_intent"
)

type PriceQuote struct {
	ID, WorkspaceID, ProjectID string
	Metric                     string
	UnitAmount                 decimal.Decimal
	Currency                   string
	Revision                   int64
	ContentHash                string
	CreatedBy                  string
	CreatedAt                  time.Time
}

func SamePriceQuoteState(left, right PriceQuote) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.Metric == right.Metric && left.UnitAmount.Equal(right.UnitAmount) &&
		left.Currency == right.Currency && left.Revision == right.Revision && left.ContentHash == right.ContentHash
}

type Estimate struct {
	ID, WorkspaceID, ProjectID   string
	BudgetPolicyID, PriceQuoteID string
	Metric, SourceType, SourceID string
	Units                        int64
	UnitAmount, TotalAmount      decimal.Decimal
	Currency                     string
	PriceQuoteRevision           int64
	BudgetPolicyRevision         int64
	BudgetLimit                  decimal.Decimal
	ContentHash, CreatedBy       string
	CreatedAt                    time.Time
}

func SameEstimateState(left, right Estimate) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.BudgetPolicyID == right.BudgetPolicyID && left.PriceQuoteID == right.PriceQuoteID &&
		left.Metric == right.Metric && left.SourceType == right.SourceType && left.SourceID == right.SourceID &&
		left.Units == right.Units && left.UnitAmount.Equal(right.UnitAmount) && left.TotalAmount.Equal(right.TotalAmount) &&
		left.Currency == right.Currency && left.PriceQuoteRevision == right.PriceQuoteRevision &&
		left.BudgetPolicyRevision == right.BudgetPolicyRevision && left.BudgetLimit.Equal(right.BudgetLimit) &&
		left.ContentHash == right.ContentHash
}
