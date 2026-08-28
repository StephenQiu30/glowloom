package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	ErrImmutableShardManifest          = errors.New("ShardManifest is immutable")
	ErrImmutableStageCandidateRevision = errors.New("StageCandidateRevision is immutable")
	ErrImmutableStageInstanceStaleness = errors.New("StageInstanceStaleness is immutable")
)

type ShardManifest struct {
	ID                 uuid.UUID         `gorm:"type:uuid;primaryKey"`
	Version            int64             `gorm:"primaryKey;not null;uniqueIndex:uq_agt_manifest_node_stage_version,priority:4;check:ck_agt_manifest_version,version >= 1"`
	WorkspaceID        uuid.UUID         `gorm:"type:uuid;not null"`
	WorkflowRunID      uuid.UUID         `gorm:"type:uuid;not null;uniqueIndex:uq_agt_manifest_node_stage_version,priority:1"`
	NodeRunID          uuid.UUID         `gorm:"type:uuid;not null;uniqueIndex:uq_agt_manifest_node_stage_version,priority:2"`
	Stage              string            `gorm:"type:varchar(40);not null;uniqueIndex:uq_agt_manifest_node_stage_version,priority:3;check:ck_agt_manifest_stage,stage IN ('extract_source_evidence','analyze_story','reconcile_story','review_storygraph','segment_episodes','analyze_episode','reconcile_episode')"`
	RootInputHash      string            `gorm:"type:char(64);not null;check:ck_agt_manifest_root_hash,char_length(root_input_hash) = 64"`
	ParentManifestHash *string           `gorm:"type:char(64);check:ck_agt_manifest_parent_hash,(version = 1 AND parent_manifest_hash IS NULL) OR (version > 1 AND char_length(parent_manifest_hash) = 64)"`
	Shards             datatypes.JSON    `gorm:"type:jsonb;not null;check:ck_agt_manifest_shards,jsonb_typeof(shards) = 'array'"`
	CoverageHash       string            `gorm:"type:char(64);not null;check:ck_agt_manifest_coverage_hash,char_length(coverage_hash) = 64"`
	ManifestHash       string            `gorm:"type:char(64);not null;uniqueIndex;check:ck_agt_manifest_hash,char_length(manifest_hash) = 64"`
	CreatedAt          time.Time         `gorm:"type:timestamptz;not null"`
	Workspace          Workspace         `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	WorkflowRun        WorkflowRun       `gorm:"foreignKey:WorkflowRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	NodeRun            NodeRunProjection `gorm:"foreignKey:NodeRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ShardManifest) TableName() string { return "agt_shard_manifests" }
func (*ShardManifest) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableShardManifest
}
func (*ShardManifest) BeforeDelete(*gorm.DB) error {
	return ErrImmutableShardManifest
}

type StageCandidateRevision struct {
	ID                          uuid.UUID               `gorm:"type:uuid;primaryKey"`
	WorkspaceID                 uuid.UUID               `gorm:"type:uuid;not null"`
	StageInstanceKey            string                  `gorm:"type:char(64);not null;uniqueIndex:uq_agt_candidate_stage_revision,priority:1;check:ck_agt_candidate_stage_key,char_length(stage_instance_key) = 64"`
	RevisionNo                  int64                   `gorm:"not null;uniqueIndex:uq_agt_candidate_stage_revision,priority:2;check:ck_agt_candidate_revision_no,revision_no >= 1"`
	ParentCandidateRevisionID   *uuid.UUID              `gorm:"type:uuid"`
	ParentCandidateRevisionHash *string                 `gorm:"type:char(64);check:ck_agt_candidate_parent_hash,parent_candidate_revision_hash IS NULL OR char_length(parent_candidate_revision_hash) = 64"`
	OriginKind                  string                  `gorm:"type:varchar(20);not null;check:ck_agt_candidate_origin_kind,origin_kind IN ('invocation','aggregate','repair');check:ck_agt_candidate_origin_union,(origin_kind = 'invocation' AND revision_no = 1 AND parent_candidate_revision_id IS NULL AND parent_candidate_revision_hash IS NULL AND invocation_origin IS NOT NULL AND aggregate_origin IS NULL AND repair_origin IS NULL AND source_invocation_id IS NOT NULL AND source_result_hash IS NOT NULL) OR (origin_kind = 'aggregate' AND revision_no = 1 AND parent_candidate_revision_id IS NULL AND parent_candidate_revision_hash IS NULL AND invocation_origin IS NULL AND aggregate_origin IS NOT NULL AND repair_origin IS NULL AND source_invocation_id IS NULL AND source_result_hash IS NULL) OR (origin_kind = 'repair' AND revision_no >= 2 AND parent_candidate_revision_id IS NOT NULL AND parent_candidate_revision_hash IS NOT NULL AND invocation_origin IS NULL AND aggregate_origin IS NULL AND repair_origin IS NOT NULL AND source_invocation_id IS NULL AND source_result_hash IS NULL)"`
	InvocationOrigin            datatypes.JSON          `gorm:"type:jsonb"`
	AggregateOrigin             datatypes.JSON          `gorm:"type:jsonb"`
	RepairOrigin                datatypes.JSON          `gorm:"type:jsonb"`
	SourceInvocationID          *uuid.UUID              `gorm:"type:uuid;uniqueIndex"`
	SourceResultHash            *string                 `gorm:"type:char(64);check:ck_agt_candidate_source_hash,source_result_hash IS NULL OR char_length(source_result_hash) = 64"`
	Candidate                   datatypes.JSON          `gorm:"type:jsonb;not null;check:ck_agt_candidate_json,jsonb_typeof(candidate) = 'object'"`
	CandidateContentHash        string                  `gorm:"type:char(64);not null;check:ck_agt_candidate_content_hash,char_length(candidate_content_hash) = 64"`
	CandidateRevisionHash       string                  `gorm:"type:char(64);not null;uniqueIndex;check:ck_agt_candidate_revision_hash,char_length(candidate_revision_hash) = 64"`
	CreatedAt                   time.Time               `gorm:"type:timestamptz;not null"`
	Workspace                   Workspace               `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	ParentCandidateRevision     *StageCandidateRevision `gorm:"foreignKey:ParentCandidateRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	SourceInvocation            *AgentInvocation        `gorm:"foreignKey:SourceInvocationID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (StageCandidateRevision) TableName() string { return "agt_stage_candidate_revisions" }
func (*StageCandidateRevision) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableStageCandidateRevision
}
func (*StageCandidateRevision) BeforeDelete(*gorm.DB) error {
	return ErrImmutableStageCandidateRevision
}

type StageCandidateHead struct {
	WorkspaceID                  uuid.UUID              `gorm:"type:uuid;not null"`
	StageInstanceKey             string                 `gorm:"type:char(64);primaryKey;check:ck_agt_candidate_head_key,char_length(stage_instance_key) = 64"`
	CurrentRevisionID            uuid.UUID              `gorm:"type:uuid;not null;uniqueIndex"`
	CurrentCandidateRevisionHash string                 `gorm:"type:char(64);not null;check:ck_agt_candidate_head_hash,char_length(current_candidate_revision_hash) = 64"`
	Revision                     int64                  `gorm:"not null;check:ck_agt_candidate_head_revision,revision >= 1"`
	UpdatedAt                    time.Time              `gorm:"type:timestamptz;not null"`
	Workspace                    Workspace              `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CurrentRevision              StageCandidateRevision `gorm:"foreignKey:CurrentRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (StageCandidateHead) TableName() string { return "agt_stage_candidate_heads" }

type StageInstanceStaleness struct {
	ID                         uuid.UUID              `gorm:"type:uuid;primaryKey"`
	WorkspaceID                uuid.UUID              `gorm:"type:uuid;not null"`
	InvocationID               *uuid.UUID             `gorm:"type:uuid;index"`
	StageInstanceKey           string                 `gorm:"type:char(64);not null;uniqueIndex;check:ck_agt_stale_stage_key,char_length(stage_instance_key) = 64"`
	CauseCandidateRevisionID   uuid.UUID              `gorm:"type:uuid;not null"`
	CauseCandidateRevisionHash string                 `gorm:"type:char(64);not null;check:ck_agt_stale_candidate_hash,char_length(cause_candidate_revision_hash) = 64"`
	CreatedAt                  time.Time              `gorm:"type:timestamptz;not null"`
	Workspace                  Workspace              `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Invocation                 *AgentInvocation       `gorm:"foreignKey:InvocationID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CauseCandidateRevision     StageCandidateRevision `gorm:"foreignKey:CauseCandidateRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (StageInstanceStaleness) TableName() string { return "agt_stage_instance_staleness" }
func (*StageInstanceStaleness) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableStageInstanceStaleness
}
func (*StageInstanceStaleness) BeforeDelete(*gorm.DB) error {
	return ErrImmutableStageInstanceStaleness
}
