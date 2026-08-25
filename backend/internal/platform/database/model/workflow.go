package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type WorkflowTask struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID   uuid.UUID      `gorm:"type:uuid;not null;index:ix_wrk_tasks_workspace_updated,priority:1"`
	TaskType      string         `gorm:"type:varchar(60);not null;index:ix_wrk_tasks_type_status,priority:1"`
	RequestType   string         `gorm:"type:varchar(60);not null;uniqueIndex:uq_wrk_task_request,priority:1"`
	RequestID     uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_wrk_task_request,priority:2"`
	Scope         datatypes.JSON `gorm:"type:jsonb;not null"`
	Status        string         `gorm:"type:varchar(30);not null;index:ix_wrk_tasks_type_status,priority:2"`
	ProgressStage string         `gorm:"type:varchar(80);not null"`
	Error         datatypes.JSON `gorm:"type:jsonb"`
	NextAction    *string        `gorm:"type:varchar(80)"`
	CancelStatus  string         `gorm:"type:varchar(20);not null"`
	Revision      int            `gorm:"not null;check:ck_wrk_task_revision,revision >= 1"`
	CreatedAt     time.Time      `gorm:"type:timestamptz;not null"`
	UpdatedAt     time.Time      `gorm:"type:timestamptz;not null;index:ix_wrk_tasks_workspace_updated,priority:2,sort:desc"`
	Workspace     Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (WorkflowTask) TableName() string { return "wrk_tasks" }
