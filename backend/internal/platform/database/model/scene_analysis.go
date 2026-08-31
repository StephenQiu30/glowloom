package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	ErrImmutableSceneAnalysisRelease   = errors.New("SceneAnalysisRelease is immutable")
	ErrImmutableDispatchAuthorization  = errors.New("SceneAnalysisDispatchAuthorization is immutable")
	ErrImmutableSceneAnalysisResult    = errors.New("SceneAnalysisResult is immutable")
	ErrImmutableSceneAnalysisCandidate = errors.New("SceneAnalysisCandidateRevision is immutable")
)

type SceneAnalysisRelease struct {
	ID                  uuid.UUID      `gorm:"type:uuid;primaryKey"`
	StageKey            string         `gorm:"type:varchar(64);not null;uniqueIndex:uq_agt_scene_release_variant,priority:1;check:ck_agt_scene_release_stage,stage_key IN ('propose_script_spans','extract_scene_facts')"`
	ProfileKey          string         `gorm:"type:varchar(64);not null;uniqueIndex:uq_agt_scene_release_variant,priority:2;check:ck_agt_scene_release_profile,profile_key = 'default'"`
	SkillReleaseID      uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_agt_scene_release_variant,priority:3"`
	SkillReleaseHash    string         `gorm:"type:char(64);not null;check:ck_agt_scene_release_skill_hash,char_length(skill_release_hash) = 64"`
	StageReleaseHash    string         `gorm:"type:char(64);not null;uniqueIndex;check:ck_agt_scene_release_stage_hash,char_length(stage_release_hash) = 64"`
	BundleContentHash   string         `gorm:"type:char(64);not null;check:ck_agt_scene_release_bundle_hash,char_length(bundle_content_hash) = 64"`
	AgentImageDigest    string         `gorm:"type:varchar(71);not null"`
	ModelCapability     string         `gorm:"type:varchar(40);not null;check:ck_agt_scene_release_capability,model_capability = 'structured_text'"`
	LoadedResourcePaths datatypes.JSON `gorm:"type:jsonb;not null;check:ck_agt_scene_release_resources,jsonb_typeof(loaded_resource_paths) = 'array'"`
	CreatedAt           time.Time      `gorm:"type:timestamptz;not null"`
}

func (SceneAnalysisRelease) TableName() string { return "agt_scene_analysis_releases" }
func (*SceneAnalysisRelease) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableSceneAnalysisRelease
}
func (*SceneAnalysisRelease) BeforeDelete(*gorm.DB) error {
	return ErrImmutableSceneAnalysisRelease
}

type SceneAnalysisControlHead struct {
	ReleaseID       uuid.UUID            `gorm:"type:uuid;primaryKey"`
	ControlRecordID uuid.UUID            `gorm:"type:uuid;not null;uniqueIndex"`
	ControlRevision int64                `gorm:"not null;check:ck_agt_scene_control_revision,control_revision >= 1"`
	Status          string               `gorm:"type:varchar(20);not null;check:ck_agt_scene_control_status,status IN ('approved','deprecated','quarantined','revoked')"`
	ControlHash     string               `gorm:"type:char(64);not null;check:ck_agt_scene_control_hash,char_length(control_hash) = 64"`
	ReleaseFence    int64                `gorm:"not null;check:ck_agt_scene_control_fence,release_fence >= 0"`
	UpdatedAt       time.Time            `gorm:"type:timestamptz;not null"`
	Release         SceneAnalysisRelease `gorm:"foreignKey:ReleaseID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (SceneAnalysisControlHead) TableName() string { return "agt_scene_analysis_control_heads" }

type SceneAnalysisInvocationRecord struct {
	ID                            uuid.UUID            `gorm:"type:uuid;primaryKey"`
	WorkspaceID                   uuid.UUID            `gorm:"type:uuid;not null"`
	ProjectID                     uuid.UUID            `gorm:"type:uuid;not null"`
	WorkflowRunID                 uuid.UUID            `gorm:"type:uuid;not null;uniqueIndex:uq_agt_scene_invocation_node_input,priority:1"`
	NodeRunID                     uuid.UUID            `gorm:"type:uuid;not null;uniqueIndex:uq_agt_scene_invocation_node_input,priority:2"`
	ReleaseID                     uuid.UUID            `gorm:"type:uuid;not null"`
	ControlRecordID               uuid.UUID            `gorm:"type:uuid;not null"`
	ControlRevision               int64                `gorm:"not null;check:ck_agt_scene_invocation_control_revision,control_revision >= 1"`
	ControlHash                   string               `gorm:"type:char(64);not null;check:ck_agt_scene_invocation_control_hash,char_length(control_hash) = 64"`
	ReleaseFence                  int64                `gorm:"not null;check:ck_agt_scene_invocation_fence,release_fence >= 0"`
	WireSchemaID                  string               `gorm:"type:varchar(64);not null;check:ck_agt_scene_invocation_wire,wire_schema_id = 'storygraph-stage-wire-production'"`
	StageKey                      string               `gorm:"type:varchar(64);not null;uniqueIndex:uq_agt_scene_invocation_node_input,priority:3;check:ck_agt_scene_invocation_stage,stage_key IN ('propose_script_spans','extract_scene_facts')"`
	ProfileKey                    string               `gorm:"type:varchar(64);not null;check:ck_agt_scene_invocation_profile,profile_key = 'default'"`
	StageInstanceKey              string               `gorm:"type:char(64);not null;uniqueIndex;check:ck_agt_scene_invocation_key,char_length(stage_instance_key) = 64"`
	InputHash                     string               `gorm:"type:char(64);not null;uniqueIndex:uq_agt_scene_invocation_node_input,priority:4;check:ck_agt_scene_invocation_input_hash,char_length(input_hash) = 64"`
	SourceVersionID               uuid.UUID            `gorm:"type:uuid;not null"`
	SourceHash                    string               `gorm:"type:char(64);not null;check:ck_agt_scene_invocation_source_hash,char_length(source_hash) = 64"`
	UpstreamCandidateRevisionID   *uuid.UUID           `gorm:"type:uuid"`
	UpstreamCandidateRevisionHash *string              `gorm:"type:char(64);check:ck_agt_scene_invocation_upstream_hash,upstream_candidate_revision_hash IS NULL OR char_length(upstream_candidate_revision_hash) = 64"`
	ShardManifestID               uuid.UUID            `gorm:"type:uuid;not null"`
	ShardManifestHash             string               `gorm:"type:char(64);not null;check:ck_agt_scene_invocation_manifest_hash,char_length(shard_manifest_hash) = 64"`
	ShardKey                      string               `gorm:"type:varchar(200);not null"`
	Payload                       datatypes.JSON       `gorm:"type:jsonb;not null;check:ck_agt_scene_invocation_payload,jsonb_typeof(payload) = 'object'"`
	Budget                        datatypes.JSON       `gorm:"type:jsonb;not null;check:ck_agt_scene_invocation_budget,jsonb_typeof(budget) = 'object'"`
	Status                        string               `gorm:"type:varchar(20);not null;index:ix_agt_scene_invocation_status_created,priority:1;check:ck_agt_scene_invocation_status,status IN ('queued','running','accepted','rejected','outcome_unknown')"`
	CreatedAt                     time.Time            `gorm:"type:timestamptz;not null;index:ix_agt_scene_invocation_status_created,priority:2"`
	UpdatedAt                     time.Time            `gorm:"type:timestamptz;not null"`
	Workspace                     Workspace            `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                       Project              `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	WorkflowRun                   WorkflowRun          `gorm:"foreignKey:WorkflowRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	NodeRun                       NodeRunProjection    `gorm:"foreignKey:NodeRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Release                       SceneAnalysisRelease `gorm:"foreignKey:ReleaseID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	SourceVersion                 DocumentRevision     `gorm:"foreignKey:SourceVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (SceneAnalysisInvocationRecord) TableName() string { return "agt_scene_analysis_invocations" }

type SceneAnalysisAttempt struct {
	ID               uuid.UUID                     `gorm:"type:uuid;primaryKey"`
	InvocationID     uuid.UUID                     `gorm:"type:uuid;not null;uniqueIndex:uq_agt_scene_attempt_claim,priority:1"`
	ClaimVersion     int64                         `gorm:"not null;uniqueIndex:uq_agt_scene_attempt_claim,priority:2;check:ck_agt_scene_attempt_claim,claim_version >= 1"`
	ControlHash      string                        `gorm:"type:char(64);not null;check:ck_agt_scene_attempt_control_hash,char_length(control_hash) = 64"`
	ReleaseFence     int64                         `gorm:"not null;check:ck_agt_scene_attempt_fence,release_fence >= 0"`
	AgentImageDigest string                        `gorm:"type:varchar(71);not null"`
	Status           string                        `gorm:"type:varchar(20);not null;check:ck_agt_scene_attempt_status,status IN ('dispatched','completed')"`
	DispatchedAt     time.Time                     `gorm:"type:timestamptz;not null"`
	CompletedAt      *time.Time                    `gorm:"type:timestamptz"`
	Invocation       SceneAnalysisInvocationRecord `gorm:"foreignKey:InvocationID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (SceneAnalysisAttempt) TableName() string { return "agt_scene_analysis_attempts" }

type SceneAnalysisDispatchAuthorization struct {
	AttemptID         uuid.UUID            `gorm:"type:uuid;primaryKey"`
	AuthorizationHash string               `gorm:"type:char(64);not null;uniqueIndex;check:ck_agt_scene_dispatch_authorization_hash,char_length(authorization_hash) = 64"`
	ExpiresAt         time.Time            `gorm:"type:timestamptz;not null"`
	IssuedAt          time.Time            `gorm:"type:timestamptz;not null"`
	Attempt           SceneAnalysisAttempt `gorm:"foreignKey:AttemptID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (SceneAnalysisDispatchAuthorization) TableName() string {
	return "agt_scene_analysis_dispatch_authorizations"
}

func (*SceneAnalysisDispatchAuthorization) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableDispatchAuthorization
}

func (*SceneAnalysisDispatchAuthorization) BeforeDelete(*gorm.DB) error {
	return ErrImmutableDispatchAuthorization
}

type SceneAnalysisResult struct {
	ID             uuid.UUID            `gorm:"type:uuid;primaryKey"`
	AttemptID      uuid.UUID            `gorm:"type:uuid;not null;uniqueIndex"`
	Status         string               `gorm:"type:varchar(20);not null;check:ck_agt_scene_result_status,status IN ('accepted','rejected','outcome_unknown')"`
	InputHash      string               `gorm:"type:char(64);not null;check:ck_agt_scene_result_input_hash,char_length(input_hash) = 64"`
	OutputHash     *string              `gorm:"type:char(64);check:ck_agt_scene_result_output_hash,output_hash IS NULL OR char_length(output_hash) = 64"`
	DiagnosticHash string               `gorm:"type:char(64);not null;check:ck_agt_scene_result_diagnostic_hash,char_length(diagnostic_hash) = 64"`
	Result         datatypes.JSON       `gorm:"type:jsonb;not null;check:ck_agt_scene_result_json,jsonb_typeof(result) = 'object'"`
	CompletedAt    time.Time            `gorm:"type:timestamptz;not null"`
	Attempt        SceneAnalysisAttempt `gorm:"foreignKey:AttemptID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (SceneAnalysisResult) TableName() string { return "agt_scene_analysis_results" }
func (*SceneAnalysisResult) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableSceneAnalysisResult
}
func (*SceneAnalysisResult) BeforeDelete(*gorm.DB) error {
	return ErrImmutableSceneAnalysisResult
}

type SceneAnalysisCandidateRevision struct {
	ID                    uuid.UUID                     `gorm:"type:uuid;primaryKey"`
	WorkspaceID           uuid.UUID                     `gorm:"type:uuid;not null"`
	ProjectID             uuid.UUID                     `gorm:"type:uuid;not null;index:ix_agt_scene_candidate_project_created,priority:1"`
	StageInstanceKey      string                        `gorm:"type:char(64);not null;uniqueIndex:uq_agt_scene_candidate_stage_revision,priority:1;check:ck_agt_scene_candidate_key,char_length(stage_instance_key) = 64"`
	RevisionNo            int64                         `gorm:"not null;uniqueIndex:uq_agt_scene_candidate_stage_revision,priority:2;check:ck_agt_scene_candidate_revision,revision_no >= 1"`
	CandidateType         string                        `gorm:"type:varchar(80);not null;check:ck_agt_scene_candidate_type,candidate_type IN ('script_span_candidate','scene_fact_candidate')"`
	SourceInvocationID    uuid.UUID                     `gorm:"type:uuid;not null;uniqueIndex"`
	SourceResultID        uuid.UUID                     `gorm:"type:uuid;not null;uniqueIndex"`
	SourceResultHash      string                        `gorm:"type:char(64);not null;check:ck_agt_scene_candidate_result_hash,char_length(source_result_hash) = 64"`
	Candidate             datatypes.JSON                `gorm:"type:jsonb;not null;check:ck_agt_scene_candidate_json,jsonb_typeof(candidate) = 'object'"`
	CandidateContentHash  string                        `gorm:"type:char(64);not null;check:ck_agt_scene_candidate_content_hash,char_length(candidate_content_hash) = 64"`
	CandidateRevisionHash string                        `gorm:"type:char(64);not null;uniqueIndex;check:ck_agt_scene_candidate_revision_hash,char_length(candidate_revision_hash) = 64"`
	CreatedAt             time.Time                     `gorm:"type:timestamptz;not null;index:ix_agt_scene_candidate_project_created,priority:2"`
	Workspace             Workspace                     `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project               Project                       `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	SourceInvocation      SceneAnalysisInvocationRecord `gorm:"foreignKey:SourceInvocationID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	SourceResult          SceneAnalysisResult           `gorm:"foreignKey:SourceResultID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (SceneAnalysisCandidateRevision) TableName() string {
	return "agt_scene_analysis_candidate_revisions"
}
func (*SceneAnalysisCandidateRevision) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableSceneAnalysisCandidate
}
func (*SceneAnalysisCandidateRevision) BeforeDelete(*gorm.DB) error {
	return ErrImmutableSceneAnalysisCandidate
}

type SceneAnalysisCandidateHead struct {
	StageInstanceKey             string                         `gorm:"type:char(64);primaryKey;check:ck_agt_scene_candidate_head_key,char_length(stage_instance_key) = 64"`
	WorkspaceID                  uuid.UUID                      `gorm:"type:uuid;not null"`
	ProjectID                    uuid.UUID                      `gorm:"type:uuid;not null"`
	CurrentRevisionID            uuid.UUID                      `gorm:"type:uuid;not null;uniqueIndex"`
	CurrentCandidateRevisionHash string                         `gorm:"type:char(64);not null;check:ck_agt_scene_candidate_head_hash,char_length(current_candidate_revision_hash) = 64"`
	Revision                     int64                          `gorm:"not null;check:ck_agt_scene_candidate_head_revision,revision >= 1"`
	UpdatedAt                    time.Time                      `gorm:"type:timestamptz;not null"`
	Workspace                    Workspace                      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                      Project                        `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CurrentRevision              SceneAnalysisCandidateRevision `gorm:"foreignKey:CurrentRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (SceneAnalysisCandidateHead) TableName() string {
	return "agt_scene_analysis_candidate_heads"
}
