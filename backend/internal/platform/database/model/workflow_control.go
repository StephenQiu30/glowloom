package model

import (
	"time"

	"github.com/google/uuid"
)

type WorkflowControlIntent struct {
	ID                  uuid.UUID   `gorm:"type:uuid;primaryKey"`
	WorkspaceID         uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_wrk_control_intent_key,priority:1"`
	WorkflowRunID       uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_wrk_control_intent_run_action,priority:1"`
	IdempotencyKey      string      `gorm:"type:varchar(200);not null;uniqueIndex:uq_wrk_control_intent_key,priority:2"`
	CommandInputHash    string      `gorm:"type:char(64);not null;check:ck_wrk_control_command_hash,char_length(command_input_hash) = 64"`
	TemporalWorkflowID  string      `gorm:"type:varchar(220);not null"`
	ControlID           uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_wrk_control_identity"`
	InputHash           string      `gorm:"type:char(64);not null;check:ck_wrk_control_input_hash,char_length(input_hash) = 64"`
	Action              string      `gorm:"type:varchar(20);not null;uniqueIndex:uq_wrk_control_intent_run_action,priority:2;check:ck_wrk_control_action,action IN ('pause','resume','cancel')"`
	ExpectedRunRevision int         `gorm:"not null;check:ck_wrk_control_expected_revision,expected_run_revision >= 1"`
	Status              string      `gorm:"type:varchar(20);not null;check:ck_wrk_control_intent_status,status IN ('pending','completed','unknown','conflict')"`
	AttemptNo           int         `gorm:"not null;check:ck_wrk_control_attempt,attempt_no >= 0"`
	Revision            int         `gorm:"not null;check:ck_wrk_control_intent_revision,revision >= 1"`
	CreatedBy           uuid.UUID   `gorm:"type:uuid;not null"`
	CreatedAt           time.Time   `gorm:"type:timestamptz;not null"`
	UpdatedAt           time.Time   `gorm:"type:timestamptz;not null"`
	Workspace           Workspace   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	WorkflowRun         WorkflowRun `gorm:"foreignKey:WorkflowRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator             UserAccount `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (WorkflowControlIntent) TableName() string { return "wrk_control_intents" }

type WorkflowControlReceipt struct {
	ID                uuid.UUID             `gorm:"type:uuid;primaryKey"`
	WorkspaceID       uuid.UUID             `gorm:"type:uuid;not null;index:ix_wrk_control_receipts_workspace_created,priority:1"`
	ControlIntentID   uuid.UUID             `gorm:"type:uuid;not null;uniqueIndex:uq_wrk_control_receipt_attempt,priority:1"`
	WorkflowRunID     uuid.UUID             `gorm:"type:uuid;not null"`
	AttemptNo         int                   `gorm:"not null;uniqueIndex:uq_wrk_control_receipt_attempt,priority:2;check:ck_wrk_control_receipt_attempt,attempt_no >= 1"`
	Outcome           string                `gorm:"type:varchar(30);not null;check:ck_wrk_control_receipt_outcome,outcome IN ('requested','applied','already_applied','unknown','conflict')"`
	ControlID         uuid.UUID             `gorm:"type:uuid;not null"`
	ExpectedInputHash string                `gorm:"type:char(64);not null;check:ck_wrk_control_expected_hash,char_length(expected_input_hash) = 64"`
	ObservedInputHash *string               `gorm:"type:char(64);check:ck_wrk_control_observed_hash,observed_input_hash IS NULL OR char_length(observed_input_hash) = 64"`
	CreatedAt         time.Time             `gorm:"type:timestamptz;not null;index:ix_wrk_control_receipts_workspace_created,priority:2,sort:desc"`
	Workspace         Workspace             `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	ControlIntent     WorkflowControlIntent `gorm:"foreignKey:ControlIntentID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	WorkflowRun       WorkflowRun           `gorm:"foreignKey:WorkflowRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (WorkflowControlReceipt) TableName() string { return "wrk_control_receipts" }
