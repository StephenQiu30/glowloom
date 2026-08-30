package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

const (
	MetricGenerationImageCall = "generation.image.call"
	MetricGenerationVideoCall = "generation.video.call"
	SourceGenerationIntent    = "generation_intent"
)

func IsBillingMetric(value string) bool {
	return value == MetricGenerationImageCall || value == MetricGenerationVideoCall
}

type PriceQuote struct {
	ID, WorkspaceID, ProjectID, ModelProfileVersionID string
	BillingMetric                                     string
	ReservationUnitAmount                             decimal.Decimal
	Currency                                          string
	Revision                                          int64
	ContentHash                                       string
	CreatedBy                                         string
	CreatedAt                                         time.Time
}

func SamePriceQuoteState(left, right PriceQuote) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.ModelProfileVersionID == right.ModelProfileVersionID && left.BillingMetric == right.BillingMetric &&
		left.ReservationUnitAmount.Equal(right.ReservationUnitAmount) &&
		left.Currency == right.Currency && left.Revision == right.Revision && left.ContentHash == right.ContentHash
}

type Estimate struct {
	ID, WorkspaceID, ProjectID                          string
	BudgetPolicyID, PriceQuoteID                        string
	ProviderBindingVersionID, ModelProfileVersionID     string
	ProviderBindingRevision, ModelProfileRevision       int64
	ProviderBindingContentHash, ModelProfileContentHash string
	PriceQuoteContentHash, Metric, SourceType, SourceID string
	Units                                               int64
	UnitAmount, TotalAmount                             decimal.Decimal
	Currency                                            string
	PriceQuoteRevision, BudgetPolicyRevision            int64
	BudgetLimit                                         decimal.Decimal
	ContentHash, CreatedBy                              string
	CreatedAt                                           time.Time
}

func SameEstimateState(left, right Estimate) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.BudgetPolicyID == right.BudgetPolicyID && left.PriceQuoteID == right.PriceQuoteID &&
		left.ProviderBindingVersionID == right.ProviderBindingVersionID &&
		left.ModelProfileVersionID == right.ModelProfileVersionID &&
		left.ProviderBindingRevision == right.ProviderBindingRevision &&
		left.ProviderBindingContentHash == right.ProviderBindingContentHash &&
		left.ModelProfileRevision == right.ModelProfileRevision &&
		left.ModelProfileContentHash == right.ModelProfileContentHash &&
		left.PriceQuoteContentHash == right.PriceQuoteContentHash &&
		left.Metric == right.Metric && left.SourceType == right.SourceType && left.SourceID == right.SourceID &&
		left.Units == right.Units && left.UnitAmount.Equal(right.UnitAmount) && left.TotalAmount.Equal(right.TotalAmount) &&
		left.Currency == right.Currency && left.PriceQuoteRevision == right.PriceQuoteRevision &&
		left.BudgetPolicyRevision == right.BudgetPolicyRevision && left.BudgetLimit.Equal(right.BudgetLimit) &&
		left.ContentHash == right.ContentHash
}
