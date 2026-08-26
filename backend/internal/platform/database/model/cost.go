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
