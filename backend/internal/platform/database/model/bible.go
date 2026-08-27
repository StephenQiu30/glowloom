package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var ErrImmutableProductionBibleVersion = errors.New("ProductionBibleVersion is immutable")

type ProductionBibleVersion struct {
	ID                    uuid.UUID              `gorm:"type:uuid;primaryKey"`
	WorkspaceID           uuid.UUID              `gorm:"type:uuid;not null"`
	ProjectID             uuid.UUID              `gorm:"type:uuid;not null;uniqueIndex:uq_scr_bible_version_project,priority:1"`
	DocumentRevisionID    uuid.UUID              `gorm:"type:uuid;not null"`
	DocumentRevisionHash  string                 `gorm:"type:char(64);not null;check:ck_scr_bible_version_document_hash,char_length(document_revision_hash) = 64"`
	CandidateRevisionID   uuid.UUID              `gorm:"type:uuid;not null;uniqueIndex"`
	CandidateRevisionNo   int64                  `gorm:"not null;check:ck_scr_bible_version_candidate_revision,candidate_revision_no >= 1"`
	CandidateRevisionHash string                 `gorm:"type:char(64);not null;check:ck_scr_bible_version_candidate_revision_hash,char_length(candidate_revision_hash) = 64"`
	CandidateContentHash  string                 `gorm:"type:char(64);not null;check:ck_scr_bible_version_candidate_content_hash,char_length(candidate_content_hash) = 64"`
	Version               int                    `gorm:"not null;uniqueIndex:uq_scr_bible_version_project,priority:2;check:ck_scr_bible_version,version >= 1"`
	ReviewDecisionID      uuid.UUID              `gorm:"type:uuid;not null;uniqueIndex"`
	Snapshot              datatypes.JSON         `gorm:"type:jsonb;not null;check:ck_scr_bible_version_snapshot,jsonb_typeof(snapshot) = 'object'"`
	ContentHash           string                 `gorm:"type:char(64);not null;uniqueIndex;check:ck_scr_bible_version_content_hash,char_length(content_hash) = 64"`
	CreatedBy             uuid.UUID              `gorm:"type:uuid;not null"`
	CreatedAt             time.Time              `gorm:"type:timestamptz;not null"`
	Workspace             Workspace              `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project               Project                `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	DocumentRevision      DocumentRevision       `gorm:"foreignKey:DocumentRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CandidateRevision     StageCandidateRevision `gorm:"foreignKey:CandidateRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	ReviewDecision        ReviewDecision         `gorm:"foreignKey:ReviewDecisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator               UserAccount            `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProductionBibleVersion) TableName() string { return "scr_production_bible_versions" }
func (*ProductionBibleVersion) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProductionBibleVersion
}
func (*ProductionBibleVersion) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProductionBibleVersion
}

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
	ID                   uuid.UUID          `gorm:"type:uuid;primaryKey"`
	WorkspaceID          uuid.UUID          `gorm:"type:uuid;not null;index:ix_agt_invocations_status_created,priority:2"`
	WorkflowRunID        *uuid.UUID         `gorm:"type:uuid;index:ix_agt_invocations_node_manifest,priority:1"`
	NodeRunID            *uuid.UUID         `gorm:"type:uuid;index:ix_agt_invocations_node_manifest,priority:2"`
	ShardManifestID      *uuid.UUID         `gorm:"type:uuid;index:ix_agt_invocations_node_manifest,priority:3"`
	ShardManifestVersion *int64             `gorm:"index:ix_agt_invocations_node_manifest,priority:4;check:ck_agt_invocation_manifest_owner,(workflow_run_id IS NULL AND node_run_id IS NULL AND shard_manifest_id IS NULL AND shard_manifest_version IS NULL) OR (workflow_run_id IS NOT NULL AND node_run_id IS NOT NULL AND shard_manifest_id IS NOT NULL AND shard_manifest_version >= 1);check:ck_agt_invocation_shard_owner,request_type NOT IN ('source_evidence_shard','story_analysis_shard','story_reconcile_shard') OR (workflow_run_id IS NOT NULL AND node_run_id IS NOT NULL AND shard_manifest_id IS NOT NULL AND shard_manifest_version >= 1)"`
	RequestType          string             `gorm:"type:varchar(40);not null;uniqueIndex:uq_agt_invocation_request,priority:1"`
	RequestID            uuid.UUID          `gorm:"type:uuid;not null;uniqueIndex:uq_agt_invocation_request,priority:2"`
	Kind                 string             `gorm:"type:varchar(40);not null;index:ix_agt_invocations_claimable,priority:2;check:ck_agt_invocation_kind,kind = 'storygraph_stage'"`
	WireSchemaVersion    string             `gorm:"type:varchar(40);not null;check:ck_agt_invocation_wire,wire_schema_version = 'storygraph-stage-wire-v1'"`
	Stage                string             `gorm:"type:varchar(40);not null;index:ix_agt_invocations_stage_shard,priority:1"`
	ShardKey             string             `gorm:"type:varchar(200);not null;index:ix_agt_invocations_stage_shard,priority:2"`
	StageInstanceKey     string             `gorm:"type:char(64);not null;uniqueIndex;check:ck_agt_invocation_stage_key,char_length(stage_instance_key) = 64"`
	ShardManifestHash    string             `gorm:"type:char(64);not null;check:ck_agt_invocation_manifest_hash,char_length(shard_manifest_hash) = 64"`
	InputHash            string             `gorm:"type:char(64);not null;check:ck_agt_invocation_input_hash,char_length(input_hash) = 64"`
	ExecutionPolicy      datatypes.JSON     `gorm:"type:jsonb;not null;check:ck_agt_invocation_execution_policy,jsonb_typeof(execution_policy) = 'object'"`
	Payload              datatypes.JSON     `gorm:"type:jsonb;not null"`
	Status               string             `gorm:"type:varchar(20);not null;index:ix_agt_invocations_status_created,priority:1;index:ix_agt_invocations_claimable,priority:1;check:ck_agt_invocation_status,status IN ('queued','running','succeeded','failed','unknown')"`
	ResultHash           *string            `gorm:"type:char(64);check:ck_agt_invocation_result_hash,result_hash IS NULL OR char_length(result_hash) = 64"`
	CandidateType        *string            `gorm:"type:varchar(80)"`
	Candidate            datatypes.JSON     `gorm:"type:jsonb"`
	Executor             datatypes.JSON     `gorm:"type:jsonb;check:ck_agt_invocation_executor,executor IS NULL OR jsonb_typeof(executor) = 'object'"`
	Error                datatypes.JSON     `gorm:"type:jsonb"`
	Attempts             int                `gorm:"not null;check:ck_agt_invocation_attempts,attempts >= 0"`
	ClaimVersion         int                `gorm:"not null;default:0;check:ck_agt_invocation_claim_version,claim_version >= 0"`
	LeaseExpiresAt       *time.Time         `gorm:"type:timestamptz;index:ix_agt_invocations_claimable,priority:3"`
	StartedAt            *time.Time         `gorm:"type:timestamptz"`
	CompletedAt          *time.Time         `gorm:"type:timestamptz"`
	CreatedAt            time.Time          `gorm:"type:timestamptz;not null;index:ix_agt_invocations_status_created,priority:3;index:ix_agt_invocations_claimable,priority:4"`
	UpdatedAt            time.Time          `gorm:"type:timestamptz;not null"`
	Workspace            Workspace          `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	WorkflowRun          *WorkflowRun       `gorm:"foreignKey:WorkflowRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	NodeRun              *NodeRunProjection `gorm:"foreignKey:NodeRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	ShardManifest        *ShardManifest     `gorm:"foreignKey:ShardManifestID,ShardManifestVersion;references:ID,Version;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (AgentInvocation) TableName() string { return "agt_invocations" }
