package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// ProductionBible is the durable Backend-owned record for one immutable script
// revision. Agent output remains a candidate JSON document until a user confirms it.
type ProductionBible struct {
	ID                  uuid.UUID        `gorm:"type:uuid;primaryKey"`
	WorkspaceID         uuid.UUID        `gorm:"type:uuid;not null;index:ix_scr_bibles_project_status_created,priority:1"`
	ProjectID           uuid.UUID        `gorm:"type:uuid;not null;index:ix_scr_bibles_project_status_created,priority:2"`
	DocumentRevisionID  uuid.UUID        `gorm:"type:uuid;not null;index:ix_scr_bibles_revision_created,priority:1"`
	TaskID              uuid.UUID        `gorm:"type:uuid;not null;uniqueIndex"`
	Status              string           `gorm:"type:varchar(30);not null;index:ix_scr_bibles_project_status_created,priority:3;check:ck_scr_bible_status,status IN ('queued','running','needs_review','confirmed','failed','unknown','superseded','cancelled')"`
	InputHash           string           `gorm:"type:char(64);not null;check:ck_scr_bible_input_hash,char_length(input_hash) = 64"`
	ResultHash          *string          `gorm:"type:char(64);check:ck_scr_bible_result_hash,result_hash IS NULL OR char_length(result_hash) = 64"`
	EngineVersion       string           `gorm:"type:varchar(80);not null"`
	ModelName           string           `gorm:"type:varchar(160);not null"`
	PromptVersion       string           `gorm:"type:varchar(80);not null"`
	SchemaVersion       string           `gorm:"type:varchar(80);not null"`
	HarnessVersion      string           `gorm:"type:varchar(80);not null"`
	CheckpointStage     *string          `gorm:"type:varchar(80)"`
	CheckpointRevision  int              `gorm:"not null;check:ck_scr_bible_checkpoint_revision,checkpoint_revision >= 0"`
	CheckpointUpdatedAt *time.Time       `gorm:"type:timestamptz"`
	Candidate           datatypes.JSON   `gorm:"type:jsonb;not null"`
	ReviewDecisions     datatypes.JSON   `gorm:"type:jsonb;not null;default:'{}'"`
	Error               datatypes.JSON   `gorm:"type:jsonb"`
	Revision            int              `gorm:"not null;check:ck_scr_bible_revision,revision >= 1"`
	ConfirmedAt         *time.Time       `gorm:"type:timestamptz"`
	ConfirmedBy         *uuid.UUID       `gorm:"type:uuid"`
	CreatedBy           uuid.UUID        `gorm:"type:uuid;not null"`
	CreatedAt           time.Time        `gorm:"type:timestamptz;not null;index:ix_scr_bibles_project_status_created,priority:4,sort:desc;index:ix_scr_bibles_revision_created,priority:2,sort:desc"`
	UpdatedAt           time.Time        `gorm:"type:timestamptz;not null"`
	Workspace           Workspace        `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project             Project          `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	DocumentRevision    DocumentRevision `gorm:"foreignKey:DocumentRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Task                WorkflowTask     `gorm:"foreignKey:TaskID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Confirmer           *UserAccount     `gorm:"foreignKey:ConfirmedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator             UserAccount      `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProductionBible) TableName() string { return "scr_production_bibles" }

// AgentInvocation is a durable Backend-owned candidate request. The private
// Agent runtime receives the signed payload and returns a candidate only.
type AgentInvocation struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID    uuid.UUID      `gorm:"type:uuid;not null;index:ix_agt_invocations_status_created,priority:2"`
	RequestType    string         `gorm:"type:varchar(40);not null;uniqueIndex:uq_agt_invocation_request,priority:1"`
	RequestID      uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_agt_invocation_request,priority:2"`
	Kind           string         `gorm:"type:varchar(40);not null;index:ix_agt_invocations_claimable,priority:2"`
	InputHash      string         `gorm:"type:char(64);not null;check:ck_agt_invocation_input_hash,char_length(input_hash) = 64"`
	Payload        datatypes.JSON `gorm:"type:jsonb;not null"`
	Status         string         `gorm:"type:varchar(20);not null;index:ix_agt_invocations_status_created,priority:1;index:ix_agt_invocations_claimable,priority:1;check:ck_agt_invocation_status,status IN ('queued','running','succeeded','failed','unknown')"`
	ResultHash     *string        `gorm:"type:char(64);check:ck_agt_invocation_result_hash,result_hash IS NULL OR char_length(result_hash) = 64"`
	Error          datatypes.JSON `gorm:"type:jsonb"`
	Attempts       int            `gorm:"not null;check:ck_agt_invocation_attempts,attempts >= 0"`
	ClaimVersion   int            `gorm:"not null;default:0;check:ck_agt_invocation_claim_version,claim_version >= 0"`
	LeaseExpiresAt *time.Time     `gorm:"type:timestamptz;index:ix_agt_invocations_claimable,priority:3"`
	StartedAt      *time.Time     `gorm:"type:timestamptz"`
	CompletedAt    *time.Time     `gorm:"type:timestamptz"`
	CreatedAt      time.Time      `gorm:"type:timestamptz;not null;index:ix_agt_invocations_status_created,priority:3;index:ix_agt_invocations_claimable,priority:4"`
	UpdatedAt      time.Time      `gorm:"type:timestamptz;not null"`
	Workspace      Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (AgentInvocation) TableName() string { return "agt_invocations" }
