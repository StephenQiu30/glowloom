package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var ErrImmutableProjectEpisodeOwner = errors.New("Project Episode Owner fact is immutable")

type EpisodePlan struct {
	ID                    uuid.UUID        `gorm:"type:uuid;primaryKey"`
	WorkspaceID           uuid.UUID        `gorm:"type:uuid;not null"`
	ProjectID             uuid.UUID        `gorm:"type:uuid;not null;index:ix_prj_episode_plans_project_created,priority:1"`
	DocumentRevisionID    uuid.UUID        `gorm:"type:uuid;not null;index:ix_prj_episode_plans_revision_created,priority:1"`
	Strategy              string           `gorm:"type:varchar(30);not null;check:ck_prj_episode_plan_strategy,strategy IN ('explicit_markers','target_duration_ai')"`
	Status                string           `gorm:"type:varchar(20);not null;check:ck_prj_episode_plan_status,status IN ('draft','review_ready','confirmed','materialized','superseded')"`
	TargetDurationMS      int              `gorm:"not null;check:ck_prj_episode_plan_duration,target_duration_ms >= 15000"`
	RequestedEpisodeCount *int             `gorm:"check:ck_prj_episode_plan_requested_count,requested_episode_count IS NULL OR requested_episode_count >= 1"`
	TotalDurationMS       int              `gorm:"not null"`
	InputHash             string           `gorm:"type:char(64);not null;check:ck_prj_episode_plan_hash,char_length(input_hash) = 64"`
	EngineVersion         string           `gorm:"type:varchar(80);not null"`
	ModelName             *string          `gorm:"type:varchar(160)"`
	PromptVersion         *string          `gorm:"type:varchar(80)"`
	SchemaVersion         string           `gorm:"type:varchar(80);not null"`
	PlanningErrorCode     *string          `gorm:"type:varchar(120)"`
	Proposals             datatypes.JSON   `gorm:"type:jsonb;not null"`
	Revision              int              `gorm:"not null;check:ck_prj_episode_plan_revision,revision >= 1"`
	ConfirmedBy           *uuid.UUID       `gorm:"type:uuid"`
	ConfirmedAt           *time.Time       `gorm:"type:timestamptz"`
	CreatedBy             uuid.UUID        `gorm:"type:uuid;not null"`
	CreatedAt             time.Time        `gorm:"type:timestamptz;not null;index:ix_prj_episode_plans_project_created,priority:2,sort:desc;index:ix_prj_episode_plans_revision_created,priority:2,sort:desc"`
	UpdatedAt             time.Time        `gorm:"type:timestamptz;not null"`
	Workspace             Workspace        `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project               Project          `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	DocumentRevision      DocumentRevision `gorm:"foreignKey:DocumentRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator               UserAccount      `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Confirmer             *UserAccount     `gorm:"foreignKey:ConfirmedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (EpisodePlan) TableName() string { return "prj_episode_plans" }

type Episode struct {
	ID                       uuid.UUID  `gorm:"type:uuid;primaryKey"`
	WorkspaceID              uuid.UUID  `gorm:"type:uuid;not null"`
	ProjectID                uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:uq_prj_episode_position,priority:1;index:ix_prj_episodes_project_status,priority:1"`
	Name                     string     `gorm:"type:varchar(120);not null"`
	Position                 int        `gorm:"not null;uniqueIndex:uq_prj_episode_position,priority:2;check:ck_prj_episode_position,position >= 1"`
	TargetDurationMS         int        `gorm:"not null;check:ck_prj_episode_duration,target_duration_ms > 0"`
	Status                   string     `gorm:"type:varchar(20);not null;index:ix_prj_episodes_project_status,priority:2;check:ck_prj_episode_status,status IN ('active','archived')"`
	Revision                 int        `gorm:"not null;check:ck_prj_episode_revision,revision >= 1"`
	CurrentScriptVersionID   *uuid.UUID `gorm:"type:uuid"`
	CurrentTimelineVersionID *uuid.UUID `gorm:"type:uuid"`
	CreatedAt                time.Time  `gorm:"type:timestamptz;not null"`
	UpdatedAt                time.Time  `gorm:"type:timestamptz;not null"`
	Workspace                Workspace  `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                  Project    `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (Episode) TableName() string { return "prj_episodes" }

type EpisodeScriptVersion struct {
	ID                 uuid.UUID        `gorm:"type:uuid;primaryKey"`
	WorkspaceID        uuid.UUID        `gorm:"type:uuid;not null"`
	ProjectID          uuid.UUID        `gorm:"type:uuid;not null"`
	EpisodeID          uuid.UUID        `gorm:"type:uuid;not null;uniqueIndex:uq_scr_episode_version,priority:1"`
	VersionNo          int              `gorm:"not null;uniqueIndex:uq_scr_episode_version,priority:2;check:ck_scr_episode_version_no,version_no >= 1"`
	DocumentRevisionID uuid.UUID        `gorm:"type:uuid;not null"`
	SourceStart        int              `gorm:"not null;check:ck_scr_episode_source_start,source_start >= 0"`
	SourceEnd          int              `gorm:"not null;check:ck_scr_episode_source_end,source_end > source_start"`
	Content            string           `gorm:"type:text;not null"`
	ContentHash        string           `gorm:"type:char(64);not null;check:ck_scr_episode_content_hash,char_length(content_hash) = 64"`
	Status             string           `gorm:"type:varchar(20);not null;check:ck_scr_episode_version_status,status IN ('draft','published')"`
	CreatedBy          uuid.UUID        `gorm:"type:uuid;not null"`
	CreatedAt          time.Time        `gorm:"type:timestamptz;not null"`
	UpdatedAt          time.Time        `gorm:"type:timestamptz;not null"`
	Workspace          Workspace        `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project            Project          `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Episode            Episode          `gorm:"foreignKey:EpisodeID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	DocumentRevision   DocumentRevision `gorm:"foreignKey:DocumentRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator            UserAccount      `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (EpisodeScriptVersion) TableName() string { return "scr_episode_versions" }

type ProjectEpisodeVersion struct {
	ID                uuid.UUID            `gorm:"type:uuid;primaryKey"`
	WorkspaceID       uuid.UUID            `gorm:"type:uuid;not null"`
	ProjectID         uuid.UUID            `gorm:"type:uuid;not null;index:ix_prj_episode_versions_project_created,priority:1"`
	EpisodeID         uuid.UUID            `gorm:"type:uuid;not null;uniqueIndex:uq_prj_episode_owner_revision,priority:1"`
	Revision          int64                `gorm:"not null;uniqueIndex:uq_prj_episode_owner_revision,priority:2;check:ck_prj_episode_owner_revision,revision >= 1"`
	ParentVersionID   *uuid.UUID           `gorm:"type:uuid"`
	ParentContentHash *string              `gorm:"type:char(64);check:ck_prj_episode_owner_parent_hash,parent_content_hash IS NULL OR char_length(parent_content_hash) = 64"`
	Status            string               `gorm:"type:varchar(20);not null;check:ck_prj_episode_owner_status,status IN ('active','archived')"`
	Position          int                  `gorm:"not null;check:ck_prj_episode_owner_position,position >= 1"`
	SequenceKey       string               `gorm:"type:varchar(20);not null"`
	Name              string               `gorm:"type:varchar(120);not null"`
	TargetDurationMS  int                  `gorm:"not null;check:ck_prj_episode_owner_duration,target_duration_ms > 0"`
	SourceVersionID   uuid.UUID            `gorm:"type:uuid;not null"`
	ScriptVersionID   uuid.UUID            `gorm:"type:uuid;not null"`
	SourceStart       int                  `gorm:"not null;check:ck_prj_episode_owner_source_start,source_start >= 0"`
	SourceEnd         int                  `gorm:"not null;check:ck_prj_episode_owner_source_end,source_end > source_start"`
	ScriptContentHash string               `gorm:"type:char(64);not null;check:ck_prj_episode_owner_script_hash,char_length(script_content_hash) = 64"`
	ContentHash       string               `gorm:"type:char(64);not null;uniqueIndex;check:ck_prj_episode_owner_hash,char_length(content_hash) = 64"`
	CreatedBy         uuid.UUID            `gorm:"type:uuid;not null"`
	CreatedAt         time.Time            `gorm:"type:timestamptz;not null;index:ix_prj_episode_versions_project_created,priority:2,sort:desc"`
	Workspace         Workspace            `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project           Project              `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Episode           Episode              `gorm:"foreignKey:EpisodeID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	SourceVersion     DocumentRevision     `gorm:"foreignKey:SourceVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	ScriptVersion     EpisodeScriptVersion `gorm:"foreignKey:ScriptVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator           UserAccount          `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProjectEpisodeVersion) TableName() string            { return "prj_episode_versions" }
func (*ProjectEpisodeVersion) BeforeUpdate(*gorm.DB) error { return ErrImmutableProjectEpisodeOwner }
func (*ProjectEpisodeVersion) BeforeDelete(*gorm.DB) error { return ErrImmutableProjectEpisodeOwner }

type ProjectEpisodeMembership struct {
	ID                 uuid.UUID             `gorm:"type:uuid;primaryKey"`
	WorkspaceID        uuid.UUID             `gorm:"type:uuid;not null"`
	ProjectID          uuid.UUID             `gorm:"type:uuid;not null;uniqueIndex:uq_prj_episode_membership_position,priority:1;uniqueIndex:uq_prj_episode_membership_episode,priority:1"`
	ScopeRevision      int64                 `gorm:"not null;uniqueIndex:uq_prj_episode_membership_position,priority:2;uniqueIndex:uq_prj_episode_membership_episode,priority:2;check:ck_prj_episode_membership_revision,scope_revision >= 1"`
	Position           int                   `gorm:"not null;uniqueIndex:uq_prj_episode_membership_position,priority:3;check:ck_prj_episode_membership_position,position >= 1"`
	EpisodeID          uuid.UUID             `gorm:"type:uuid;not null;uniqueIndex:uq_prj_episode_membership_episode,priority:3"`
	EpisodeVersionID   uuid.UUID             `gorm:"type:uuid;not null;index:ix_prj_episode_memberships_version"`
	VersionContentHash string                `gorm:"type:char(64);not null;check:ck_prj_episode_membership_hash,char_length(version_content_hash) = 64"`
	CreatedAt          time.Time             `gorm:"type:timestamptz;not null"`
	Workspace          Workspace             `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project            Project               `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Episode            Episode               `gorm:"foreignKey:EpisodeID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	EpisodeVersion     ProjectEpisodeVersion `gorm:"foreignKey:EpisodeVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProjectEpisodeMembership) TableName() string            { return "prj_episode_memberships" }
func (*ProjectEpisodeMembership) BeforeUpdate(*gorm.DB) error { return ErrImmutableProjectEpisodeOwner }
func (*ProjectEpisodeMembership) BeforeDelete(*gorm.DB) error { return ErrImmutableProjectEpisodeOwner }

type ProjectEpisodeScopeHead struct {
	ProjectID          uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID        uuid.UUID      `gorm:"type:uuid;not null"`
	ScopeKey           string         `gorm:"type:varchar(160);not null"`
	ScopeRevision      int64          `gorm:"not null;check:ck_prj_episode_head_scope_revision,scope_revision >= 1"`
	ScopeContentHash   string         `gorm:"type:char(64);not null"`
	MemberCount        int64          `gorm:"not null;check:ck_prj_episode_head_member_count,member_count >= 1"`
	MembersHash        string         `gorm:"type:char(64);not null"`
	CollectionRootHash string         `gorm:"type:char(64);not null"`
	CurrentVersionRefs datatypes.JSON `gorm:"type:jsonb;not null;check:ck_prj_episode_head_refs,jsonb_typeof(current_version_refs) = 'array'"`
	HeadRevision       int64          `gorm:"not null;check:ck_prj_episode_head_revision,head_revision >= 1"`
	HeadContentHash    string         `gorm:"type:char(64);not null"`
	UpdatedAt          time.Time      `gorm:"type:timestamptz;not null"`
	Workspace          Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project            Project        `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProjectEpisodeScopeHead) TableName() string { return "prj_episode_scope_heads" }

type ProjectEpisodeCollectionReceipt struct {
	ID                        uuid.UUID      `gorm:"type:uuid;primaryKey"`
	CommandID                 uuid.UUID      `gorm:"type:uuid;not null"`
	IdempotencyKey            string         `gorm:"type:varchar(200);not null"`
	WorkspaceID               uuid.UUID      `gorm:"type:uuid;not null"`
	ProjectID                 uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_prj_episode_collection_receipt,priority:2"`
	DecisionCheckpointID      string         `gorm:"type:varchar(80);not null;uniqueIndex:uq_prj_episode_collection_receipt,priority:1"`
	ReviewDecisionID          uuid.UUID      `gorm:"type:uuid;not null"`
	OwnerKind                 string         `gorm:"type:varchar(80);not null;uniqueIndex:uq_prj_episode_collection_receipt,priority:3"`
	VersionFamily             string         `gorm:"type:varchar(80);not null;uniqueIndex:uq_prj_episode_collection_receipt,priority:4"`
	ScopeKind                 string         `gorm:"type:varchar(40);not null;uniqueIndex:uq_prj_episode_collection_receipt,priority:5"`
	ScopeKey                  string         `gorm:"type:varchar(160);not null;uniqueIndex:uq_prj_episode_collection_receipt,priority:6"`
	ScopeRevision             int64          `gorm:"not null;check:ck_prj_episode_collection_scope_revision,scope_revision >= 1"`
	ScopeContentHash          string         `gorm:"type:char(64);not null"`
	Members                   datatypes.JSON `gorm:"type:jsonb;not null;check:ck_prj_episode_collection_members,jsonb_typeof(members) = 'array'"`
	MemberCount               int64          `gorm:"not null;check:ck_prj_episode_collection_count,member_count >= 1"`
	MembersHash               string         `gorm:"type:char(64);not null"`
	CollectionRootHash        string         `gorm:"type:char(64);not null;uniqueIndex:uq_prj_episode_collection_receipt,priority:7"`
	CoveredScopeKeys          datatypes.JSON `gorm:"type:jsonb;not null;check:ck_prj_episode_collection_scopes,jsonb_typeof(covered_scope_keys) = 'array'"`
	CommittedOwnerVersionRefs datatypes.JSON `gorm:"type:jsonb;not null;check:ck_prj_episode_collection_refs,jsonb_typeof(committed_owner_version_refs) = 'array'"`
	ReceiptContentHash        string         `gorm:"type:char(64);not null;uniqueIndex"`
	CommittedAt               time.Time      `gorm:"type:timestamptz;not null"`
	CommittedBy               uuid.UUID      `gorm:"type:uuid;not null"`
	Workspace                 Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                   Project        `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Reviewer                  UserAccount    `gorm:"foreignKey:CommittedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProjectEpisodeCollectionReceipt) TableName() string { return "prj_episode_collection_receipts" }
func (*ProjectEpisodeCollectionReceipt) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProjectEpisodeOwner
}
func (*ProjectEpisodeCollectionReceipt) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProjectEpisodeOwner
}

type EpisodeStructure struct {
	ID              uuid.UUID            `gorm:"type:uuid;primaryKey"`
	WorkspaceID     uuid.UUID            `gorm:"type:uuid;not null"`
	ProjectID       uuid.UUID            `gorm:"type:uuid;not null"`
	EpisodeID       uuid.UUID            `gorm:"type:uuid;not null;index:ix_scr_structures_episode_created,priority:1"`
	ScriptVersionID uuid.UUID            `gorm:"type:uuid;not null;uniqueIndex"`
	Status          string               `gorm:"type:varchar(20);not null;check:ck_scr_structure_status,status IN ('needs_review','confirmed','superseded')"`
	Scenes          datatypes.JSON       `gorm:"type:jsonb;not null"`
	ResultHash      string               `gorm:"type:char(64);not null;check:ck_scr_structure_hash,char_length(result_hash) = 64"`
	Revision        int                  `gorm:"not null;check:ck_scr_structure_revision,revision >= 1"`
	ConfirmedBy     *uuid.UUID           `gorm:"type:uuid"`
	ConfirmedAt     *time.Time           `gorm:"type:timestamptz"`
	CreatedBy       uuid.UUID            `gorm:"type:uuid;not null"`
	CreatedAt       time.Time            `gorm:"type:timestamptz;not null;index:ix_scr_structures_episode_created,priority:2,sort:desc"`
	UpdatedAt       time.Time            `gorm:"type:timestamptz;not null"`
	Workspace       Workspace            `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project         Project              `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Episode         Episode              `gorm:"foreignKey:EpisodeID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	ScriptVersion   EpisodeScriptVersion `gorm:"foreignKey:ScriptVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Creator         UserAccount          `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Confirmer       *UserAccount         `gorm:"foreignKey:ConfirmedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (EpisodeStructure) TableName() string { return "scr_episode_structures" }

type ImportCommit struct {
	ID                      uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID             uuid.UUID      `gorm:"type:uuid;not null"`
	ProjectID               uuid.UUID      `gorm:"type:uuid;not null"`
	PlanID                  uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex"`
	Mode                    string         `gorm:"type:varchar(20);not null"`
	Status                  string         `gorm:"type:varchar(20);not null;check:ck_prj_import_commit_status,status IN ('pending','materializing','materialized','publishing','published','conflict','failed')"`
	InputHash               string         `gorm:"type:char(64);not null;check:ck_prj_import_commit_hash,char_length(input_hash) = 64"`
	ExpectedProjectRevision int            `gorm:"not null"`
	ExpectedActiveOrderHash string         `gorm:"type:char(64);not null"`
	ErrorCode               *string        `gorm:"type:varchar(120)"`
	Segments                datatypes.JSON `gorm:"type:jsonb;not null"`
	Revision                int            `gorm:"not null;check:ck_prj_import_commit_revision,revision >= 1"`
	CreatedBy               uuid.UUID      `gorm:"type:uuid;not null"`
	CreatedAt               time.Time      `gorm:"type:timestamptz;not null"`
	UpdatedAt               time.Time      `gorm:"type:timestamptz;not null"`
	Workspace               Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                 Project        `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Plan                    EpisodePlan    `gorm:"foreignKey:PlanID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator                 UserAccount    `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ImportCommit) TableName() string { return "prj_import_commits" }
