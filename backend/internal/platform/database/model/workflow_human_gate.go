package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var ErrImmutableWorkflowHumanGateInput = errors.New("WorkflowHumanGateInput is immutable")

// WorkflowHumanGateInput is the immutable review fact frozen before a HumanTask is opened.
type WorkflowHumanGateInput struct {
	ID                uuid.UUID         `gorm:"type:uuid;primaryKey"`
	WorkspaceID       uuid.UUID         `gorm:"type:uuid;not null;index:ix_wrk_human_gate_inputs_workspace_created,priority:1"`
	ProjectID         uuid.UUID         `gorm:"type:uuid;not null;index:ix_wrk_human_gate_inputs_project_created,priority:1"`
	WorkflowRunID     uuid.UUID         `gorm:"type:uuid;not null;index:ix_wrk_human_gate_inputs_run"`
	NodeRunID         uuid.UUID         `gorm:"type:uuid;not null;uniqueIndex:uq_wrk_human_gate_input_node"`
	GateKey           string            `gorm:"type:varchar(80);not null;check:ck_wrk_human_gate_input_key,gate_key IN ('structure_identity','bible_continuity','visual_foundation_scope')"`
	GateInstanceKey   string            `gorm:"type:varchar(220);not null;uniqueIndex:uq_wrk_human_gate_input_instance"`
	SubjectType       string            `gorm:"type:varchar(80);not null;check:ck_wrk_human_gate_input_subject,subject_type IN ('structure_identity','production_world','visual_foundation_scope')"`
	SubjectHash       string            `gorm:"type:char(64);not null;check:ck_wrk_human_gate_input_subject_hash,char_length(subject_hash) = 64"`
	EffectPlanHash    string            `gorm:"type:char(64);not null;check:ck_wrk_human_gate_input_effect_hash,char_length(effect_plan_hash) = 64"`
	InputHash         string            `gorm:"type:char(64);not null;uniqueIndex:uq_wrk_human_gate_input_hash;check:ck_wrk_human_gate_input_hash,char_length(input_hash) = 64"`
	Input             datatypes.JSON    `gorm:"type:jsonb;not null;check:ck_wrk_human_gate_input_json,jsonb_typeof(input) = 'object'"`
	CreatedAt         time.Time         `gorm:"type:timestamptz;not null;index:ix_wrk_human_gate_inputs_workspace_created,priority:2,sort:desc;index:ix_wrk_human_gate_inputs_project_created,priority:2,sort:desc"`
	Workspace         Workspace         `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project           Project           `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	WorkflowRun       WorkflowRun       `gorm:"foreignKey:WorkflowRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	NodeRunProjection NodeRunProjection `gorm:"foreignKey:NodeRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (WorkflowHumanGateInput) TableName() string { return "wrk_human_gate_inputs" }

func (*WorkflowHumanGateInput) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableWorkflowHumanGateInput
}

func (*WorkflowHumanGateInput) BeforeDelete(*gorm.DB) error {
	return ErrImmutableWorkflowHumanGateInput
}
