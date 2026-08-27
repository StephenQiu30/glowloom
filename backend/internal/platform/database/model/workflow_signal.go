package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type WorkflowHumanGateApplyReceipt struct {
	ID               uuid.UUID         `gorm:"type:uuid;primaryKey"`
	WorkspaceID      uuid.UUID         `gorm:"type:uuid;not null;index:ix_wrk_gate_apply_workspace_created,priority:1"`
	WorkflowRunID    uuid.UUID         `gorm:"type:uuid;not null"`
	NodeRunID        uuid.UUID         `gorm:"type:uuid;not null"`
	HumanTaskID      uuid.UUID         `gorm:"type:uuid;not null"`
	ReviewDecisionID uuid.UUID         `gorm:"type:uuid;not null;uniqueIndex:uq_wrk_gate_apply_decision"`
	SubjectRevision  int               `gorm:"not null;check:ck_wrk_gate_apply_subject_revision,subject_revision >= 1"`
	Decision         string            `gorm:"type:varchar(30);not null;check:ck_wrk_gate_apply_decision,decision IN ('approved','rejected','changes_requested','selected')"`
	Status           string            `gorm:"type:varchar(20);not null;check:ck_wrk_gate_apply_status,status IN ('not_required','completed','conflict');check:ck_wrk_gate_apply_owner_evidence,(status = 'completed' AND decision IN ('approved','selected') AND owner_receipt_id IS NOT NULL AND owner_operation IS NOT NULL AND output IS NOT NULL AND output_hash IS NOT NULL AND conflict_code IS NULL) OR (status = 'not_required' AND decision IN ('rejected','changes_requested') AND owner_receipt_id IS NULL AND owner_operation IS NULL AND output IS NULL AND output_hash IS NULL AND conflict_code IS NULL) OR (status = 'conflict' AND decision IN ('approved','selected') AND owner_receipt_id IS NULL AND owner_operation IS NULL AND output IS NULL AND output_hash IS NULL AND conflict_code IS NOT NULL)"`
	ConflictCode     *string           `gorm:"type:varchar(80)"`
	OwnerReceiptID   *uuid.UUID        `gorm:"type:uuid;uniqueIndex:uq_wrk_gate_apply_owner_receipt"`
	OwnerOperation   *string           `gorm:"type:varchar(120)"`
	Output           datatypes.JSON    `gorm:"type:jsonb"`
	OutputHash       *string           `gorm:"type:char(64);check:ck_wrk_gate_apply_output_hash,output_hash IS NULL OR char_length(output_hash) = 64"`
	CreatedBy        uuid.UUID         `gorm:"type:uuid;not null"`
	CreatedAt        time.Time         `gorm:"type:timestamptz;not null;index:ix_wrk_gate_apply_workspace_created,priority:2,sort:desc"`
	Workspace        Workspace         `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	WorkflowRun      WorkflowRun       `gorm:"foreignKey:WorkflowRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	NodeRun          NodeRunProjection `gorm:"foreignKey:NodeRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	HumanTask        HumanTask         `gorm:"foreignKey:HumanTaskID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	ReviewDecision   ReviewDecision    `gorm:"foreignKey:ReviewDecisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator          UserAccount       `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (WorkflowHumanGateApplyReceipt) TableName() string { return "wrk_human_gate_apply_receipts" }

type WorkflowSignalIntent struct {
	ID                 uuid.UUID                     `gorm:"type:uuid;primaryKey"`
	WorkspaceID        uuid.UUID                     `gorm:"type:uuid;not null;uniqueIndex:uq_wrk_signal_intent_key,priority:1"`
	WorkflowRunID      uuid.UUID                     `gorm:"type:uuid;not null"`
	NodeRunID          uuid.UUID                     `gorm:"type:uuid;not null"`
	HumanTaskID        uuid.UUID                     `gorm:"type:uuid;not null"`
	ReviewDecisionID   uuid.UUID                     `gorm:"type:uuid;not null;uniqueIndex:uq_wrk_signal_intent_decision"`
	ApplyReceiptID     uuid.UUID                     `gorm:"type:uuid;not null;uniqueIndex:uq_wrk_signal_intent_apply"`
	IdempotencyKey     string                        `gorm:"type:varchar(200);not null;uniqueIndex:uq_wrk_signal_intent_key,priority:2"`
	CommandInputHash   string                        `gorm:"type:char(64);not null;check:ck_wrk_signal_command_hash,char_length(command_input_hash) = 64"`
	TemporalWorkflowID string                        `gorm:"type:varchar(220);not null"`
	SignalID           string                        `gorm:"type:uuid;not null;uniqueIndex:uq_wrk_signal_id"`
	InputHash          string                        `gorm:"type:char(64);not null;check:ck_wrk_signal_input_hash,char_length(input_hash) = 64"`
	Decision           string                        `gorm:"type:varchar(30);not null;check:ck_wrk_signal_decision,decision IN ('approved','rejected','changes_requested','selected')"`
	SubjectRevision    int                           `gorm:"not null;check:ck_wrk_signal_subject_revision,subject_revision >= 1"`
	Status             string                        `gorm:"type:varchar(20);not null;check:ck_wrk_signal_intent_status,status IN ('pending','completed','unknown','conflict')"`
	AttemptNo          int                           `gorm:"not null;check:ck_wrk_signal_attempt,attempt_no >= 0"`
	Revision           int                           `gorm:"not null;check:ck_wrk_signal_intent_revision,revision >= 1"`
	CreatedBy          uuid.UUID                     `gorm:"type:uuid;not null"`
	CreatedAt          time.Time                     `gorm:"type:timestamptz;not null"`
	UpdatedAt          time.Time                     `gorm:"type:timestamptz;not null"`
	Workspace          Workspace                     `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	WorkflowRun        WorkflowRun                   `gorm:"foreignKey:WorkflowRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	NodeRun            NodeRunProjection             `gorm:"foreignKey:NodeRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	HumanTask          HumanTask                     `gorm:"foreignKey:HumanTaskID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	ReviewDecision     ReviewDecision                `gorm:"foreignKey:ReviewDecisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	ApplyReceipt       WorkflowHumanGateApplyReceipt `gorm:"foreignKey:ApplyReceiptID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator            UserAccount                   `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (WorkflowSignalIntent) TableName() string { return "wrk_signal_intents" }

type WorkflowSignalReceipt struct {
	ID                uuid.UUID            `gorm:"type:uuid;primaryKey"`
	WorkspaceID       uuid.UUID            `gorm:"type:uuid;not null;index:ix_wrk_signal_receipts_workspace_created,priority:1"`
	SignalIntentID    uuid.UUID            `gorm:"type:uuid;not null;uniqueIndex:uq_wrk_signal_receipt_attempt,priority:1"`
	WorkflowRunID     uuid.UUID            `gorm:"type:uuid;not null"`
	AttemptNo         int                  `gorm:"not null;uniqueIndex:uq_wrk_signal_receipt_attempt,priority:2;check:ck_wrk_signal_receipt_attempt,attempt_no >= 1"`
	Outcome           string               `gorm:"type:varchar(30);not null;check:ck_wrk_signal_receipt_outcome,outcome IN ('signaled','already_applied','unknown','conflict')"`
	SignalID          string               `gorm:"type:uuid;not null"`
	ExpectedInputHash string               `gorm:"type:char(64);not null;check:ck_wrk_signal_expected_hash,char_length(expected_input_hash) = 64"`
	ObservedInputHash *string              `gorm:"type:char(64);check:ck_wrk_signal_observed_hash,observed_input_hash IS NULL OR char_length(observed_input_hash) = 64"`
	CreatedAt         time.Time            `gorm:"type:timestamptz;not null;index:ix_wrk_signal_receipts_workspace_created,priority:2,sort:desc"`
	Workspace         Workspace            `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	SignalIntent      WorkflowSignalIntent `gorm:"foreignKey:SignalIntentID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	WorkflowRun       WorkflowRun          `gorm:"foreignKey:WorkflowRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (WorkflowSignalReceipt) TableName() string { return "wrk_signal_receipts" }
