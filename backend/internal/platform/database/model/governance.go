package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type CommandReceipt struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID    uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_sys_command_receipt_scope,priority:1"`
	Operation      string         `gorm:"type:varchar(80);not null;uniqueIndex:uq_sys_command_receipt_scope,priority:2"`
	IdempotencyKey string         `gorm:"type:varchar(200);not null;uniqueIndex:uq_sys_command_receipt_scope,priority:3"`
	InputHash      string         `gorm:"type:char(64);not null;check:ck_sys_command_receipt_hash,char_length(input_hash) = 64"`
	ResourceID     uuid.UUID      `gorm:"type:uuid;not null;index:ix_sys_command_receipt_resource"`
	Result         datatypes.JSON `gorm:"type:jsonb;not null"`
	CreatedBy      uuid.UUID      `gorm:"type:uuid;not null"`
	CreatedAt      time.Time      `gorm:"type:timestamptz;not null"`
	Workspace      Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator        UserAccount    `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (CommandReceipt) TableName() string { return "sys_command_receipts" }

type AuditEvent struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID uuid.UUID      `gorm:"type:uuid;not null;index:ix_gov_audit_workspace_occurred,priority:1;index:ix_gov_audit_workspace_target_occurred,priority:1;index:ix_gov_audit_workspace_action_occurred,priority:1;index:ix_gov_audit_workspace_actor_occurred,priority:1"`
	ActorID     uuid.UUID      `gorm:"type:uuid;not null;index:ix_gov_audit_workspace_actor_occurred,priority:2"`
	Action      string         `gorm:"type:varchar(80);not null;index:ix_gov_audit_workspace_action_occurred,priority:2"`
	TargetType  string         `gorm:"type:varchar(60);not null;index:ix_gov_audit_workspace_target_occurred,priority:2"`
	TargetID    uuid.UUID      `gorm:"type:uuid;not null;index:ix_gov_audit_workspace_target_occurred,priority:3"`
	Result      string         `gorm:"type:varchar(20);not null;check:ck_gov_audit_result,result IN ('succeeded','denied','failed')"`
	TraceID     string         `gorm:"type:varchar(64);not null"`
	Metadata    datatypes.JSON `gorm:"type:jsonb;not null"`
	OccurredAt  time.Time      `gorm:"type:timestamptz;not null;index:ix_gov_audit_workspace_occurred,priority:2,sort:desc;index:ix_gov_audit_workspace_target_occurred,priority:4,sort:desc;index:ix_gov_audit_workspace_action_occurred,priority:3,sort:desc;index:ix_gov_audit_workspace_actor_occurred,priority:3,sort:desc"`
	Workspace   Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Actor       UserAccount    `gorm:"foreignKey:ActorID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (AuditEvent) TableName() string { return "gov_audit_events" }
