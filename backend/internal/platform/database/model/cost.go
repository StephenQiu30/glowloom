package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type CostBudgetPolicy struct {
	ID          uuid.UUID       `gorm:"type:uuid;primaryKey"`
	WorkspaceID uuid.UUID       `gorm:"type:uuid;not null;index"`
	ProjectID   uuid.UUID       `gorm:"type:uuid;not null;uniqueIndex:uq_cst_budget_project"`
	LimitAmount decimal.Decimal `gorm:"type:numeric(20,6);not null;check:ck_cst_budget_limit,limit_amount >= 0"`
	Currency    string          `gorm:"type:char(3);not null;check:ck_cst_budget_currency,currency ~ '^[A-Z]{3}$'"`
	Revision    int64           `gorm:"not null;check:ck_cst_budget_revision,revision >= 1"`
	ContentHash string          `gorm:"type:char(64);not null;check:ck_cst_budget_content_hash,char_length(content_hash) = 64"`
	CreatedBy   uuid.UUID       `gorm:"type:uuid;not null"`
	UpdatedBy   uuid.UUID       `gorm:"type:uuid;not null"`
	CreatedAt   time.Time       `gorm:"type:timestamptz;not null"`
	UpdatedAt   time.Time       `gorm:"type:timestamptz;not null"`
	Workspace   Workspace       `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project     Project         `gorm:"foreignKey:ProjectID,WorkspaceID;references:ID,WorkspaceID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator     UserAccount     `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Updater     UserAccount     `gorm:"foreignKey:UpdatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (CostBudgetPolicy) TableName() string { return "cst_budget_policies" }

type CostPriceQuote struct {
	ID          uuid.UUID       `gorm:"type:uuid;primaryKey"`
	WorkspaceID uuid.UUID       `gorm:"type:uuid;not null;index"`
	ProjectID   uuid.UUID       `gorm:"type:uuid;not null;uniqueIndex:uq_cst_price_quote_revision,priority:1"`
	Metric      string          `gorm:"type:varchar(64);not null;uniqueIndex:uq_cst_price_quote_revision,priority:2;check:ck_cst_price_quote_metric,metric = 'generation.image'"`
	UnitAmount  decimal.Decimal `gorm:"type:numeric(20,6);not null;check:ck_cst_price_quote_amount,unit_amount > 0"`
	Currency    string          `gorm:"type:char(3);not null;check:ck_cst_price_quote_currency,currency ~ '^[A-Z]{3}$'"`
	Revision    int64           `gorm:"not null;uniqueIndex:uq_cst_price_quote_revision,priority:3;check:ck_cst_price_quote_revision,revision >= 1"`
	ContentHash string          `gorm:"type:char(64);not null;check:ck_cst_price_quote_content_hash,char_length(content_hash) = 64"`
	CreatedBy   uuid.UUID       `gorm:"type:uuid;not null"`
	CreatedAt   time.Time       `gorm:"type:timestamptz;not null"`
	Workspace   Workspace       `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project     Project         `gorm:"foreignKey:ProjectID,WorkspaceID;references:ID,WorkspaceID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator     UserAccount     `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (CostPriceQuote) TableName() string { return "cst_price_quotes" }

type CostEstimate struct {
	ID                   uuid.UUID        `gorm:"type:uuid;primaryKey"`
	WorkspaceID          uuid.UUID        `gorm:"type:uuid;not null;index"`
	ProjectID            uuid.UUID        `gorm:"type:uuid;not null;uniqueIndex:uq_cst_estimate_source,priority:1"`
	BudgetPolicyID       uuid.UUID        `gorm:"type:uuid;not null"`
	PriceQuoteID         uuid.UUID        `gorm:"type:uuid;not null"`
	Metric               string           `gorm:"type:varchar(64);not null;check:ck_cst_estimate_metric,metric = 'generation.image'"`
	SourceType           string           `gorm:"type:varchar(64);not null;uniqueIndex:uq_cst_estimate_source,priority:2;check:ck_cst_estimate_source_type,source_type = 'generation_intent'"`
	SourceID             uuid.UUID        `gorm:"type:uuid;not null;uniqueIndex:uq_cst_estimate_source,priority:3"`
	Units                int64            `gorm:"not null;check:ck_cst_estimate_units,units > 0"`
	UnitAmount           decimal.Decimal  `gorm:"type:numeric(20,6);not null;check:ck_cst_estimate_unit_amount,unit_amount > 0"`
	TotalAmount          decimal.Decimal  `gorm:"type:numeric(20,6);not null;check:ck_cst_estimate_total_amount,total_amount > 0"`
	Currency             string           `gorm:"type:char(3);not null;check:ck_cst_estimate_currency,currency ~ '^[A-Z]{3}$'"`
	PriceQuoteRevision   int64            `gorm:"not null;check:ck_cst_estimate_price_revision,price_quote_revision >= 1"`
	BudgetPolicyRevision int64            `gorm:"not null;check:ck_cst_estimate_budget_revision,budget_policy_revision >= 1"`
	BudgetLimit          decimal.Decimal  `gorm:"type:numeric(20,6);not null;check:ck_cst_estimate_budget_limit,budget_limit >= 0"`
	ContentHash          string           `gorm:"type:char(64);not null;check:ck_cst_estimate_content_hash,char_length(content_hash) = 64"`
	CreatedBy            uuid.UUID        `gorm:"type:uuid;not null"`
	CreatedAt            time.Time        `gorm:"type:timestamptz;not null"`
	Workspace            Workspace        `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project              Project          `gorm:"foreignKey:ProjectID,WorkspaceID;references:ID,WorkspaceID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	BudgetPolicy         CostBudgetPolicy `gorm:"foreignKey:BudgetPolicyID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	PriceQuote           CostPriceQuote   `gorm:"foreignKey:PriceQuoteID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator              UserAccount      `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (CostEstimate) TableName() string { return "cst_estimates" }
