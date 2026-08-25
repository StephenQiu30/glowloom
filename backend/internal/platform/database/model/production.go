package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Project struct {
	ID               uuid.UUID       `gorm:"type:uuid;primaryKey;uniqueIndex:uq_prj_project_id_workspace,priority:1"`
	WorkspaceID      uuid.UUID       `gorm:"type:uuid;not null;uniqueIndex:uq_prj_project_id_workspace,priority:2;index:ix_prj_project_workspace_status_updated,priority:1"`
	Name             string          `gorm:"type:varchar(120);not null"`
	Description      *string         `gorm:"type:text"`
	AspectRatio      string          `gorm:"type:varchar(10);not null"`
	Language         string          `gorm:"type:varchar(35);not null"`
	VisualStyle      *string         `gorm:"type:varchar(200)"`
	TargetDurationMS int             `gorm:"not null;check:ck_prj_project_duration,target_duration_ms > 0"`
	BudgetLimit      decimal.Decimal `gorm:"type:numeric(20,6);not null;check:ck_prj_project_budget,budget_limit >= 0"`
	Currency         string          `gorm:"type:varchar(3);not null"`
	Status           string          `gorm:"type:varchar(20);not null;index:ix_prj_project_workspace_status_updated,priority:2;check:ck_prj_project_status,status IN ('active','archived')"`
	Revision         int             `gorm:"not null;check:ck_prj_project_revision,revision >= 1"`
	ArchivedAt       *time.Time      `gorm:"type:timestamptz"`
	ArchivedBy       *uuid.UUID      `gorm:"type:uuid"`
	CreatedAt        time.Time       `gorm:"type:timestamptz;not null"`
	UpdatedAt        time.Time       `gorm:"type:timestamptz;not null;index:ix_prj_project_workspace_status_updated,priority:3,sort:desc"`
	Workspace        Workspace       `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Archiver         *UserAccount    `gorm:"foreignKey:ArchivedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (Project) TableName() string { return "prj_projects" }
