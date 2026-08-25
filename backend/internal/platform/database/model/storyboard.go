package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type StoryboardDraftSet struct {
	ID                   uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID          uuid.UUID      `gorm:"type:uuid;not null"`
	ProjectID            uuid.UUID      `gorm:"type:uuid;not null"`
	StructureCommitID    uuid.UUID      `gorm:"type:uuid;not null;index"`
	StructureRevision    int            `gorm:"not null;check:ck_stb_draft_set_structure_revision,structure_revision >= 1"`
	StructureContentHash string         `gorm:"type:char(64);not null;check:ck_stb_draft_set_structure_hash,char_length(structure_content_hash) = 64"`
	Status               string         `gorm:"type:varchar(20);not null;check:ck_stb_draft_set_status,status IN ('queued','needs_review','failed','unknown','cancelled','applied')"`
	InputHash            string         `gorm:"type:char(64);not null;check:ck_stb_draft_set_input_hash,char_length(input_hash) = 64"`
	ResultHash           *string        `gorm:"type:char(64);check:ck_stb_draft_set_result_hash,result_hash IS NULL OR char_length(result_hash) = 64"`
	Batches              datatypes.JSON `gorm:"type:jsonb;not null"`
	Revision             int            `gorm:"not null;check:ck_stb_draft_set_revision,revision >= 1"`
	CreatedBy            uuid.UUID      `gorm:"type:uuid;not null"`
	CreatedAt            time.Time      `gorm:"type:timestamptz;not null"`
	UpdatedAt            time.Time      `gorm:"type:timestamptz;not null"`
	Workspace            Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project              Project        `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	StructureCommit      ImportCommit   `gorm:"foreignKey:StructureCommitID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator              UserAccount    `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (StoryboardDraftSet) TableName() string { return "stb_draft_sets" }

type StoryboardDraftBatch struct {
	ID              uuid.UUID            `gorm:"type:uuid;primaryKey"`
	WorkspaceID     uuid.UUID            `gorm:"type:uuid;not null"`
	ProjectID       uuid.UUID            `gorm:"type:uuid;not null"`
	EpisodeID       uuid.UUID            `gorm:"type:uuid;not null;index:ix_stb_batches_episode_created,priority:1"`
	StructureID     uuid.UUID            `gorm:"type:uuid;not null"`
	ScriptVersionID uuid.UUID            `gorm:"type:uuid;not null"`
	TaskID          uuid.UUID            `gorm:"type:uuid;not null;uniqueIndex"`
	Status          string               `gorm:"type:varchar(20);not null;check:ck_stb_batch_status,status IN ('queued','running','needs_review','approved','applied','failed','unknown','cancelled')"`
	InputHash       string               `gorm:"type:char(64);not null;check:ck_stb_batch_input_hash,char_length(input_hash) = 64"`
	ResultHash      *string              `gorm:"type:char(64);check:ck_stb_batch_result_hash,result_hash IS NULL OR char_length(result_hash) = 64"`
	Candidate       datatypes.JSON       `gorm:"type:jsonb;not null"`
	Decisions       datatypes.JSON       `gorm:"type:jsonb;not null"`
	Error           datatypes.JSON       `gorm:"type:jsonb"`
	Revision        int                  `gorm:"not null;check:ck_stb_batch_revision,revision >= 1"`
	ApprovedBy      *uuid.UUID           `gorm:"type:uuid"`
	ApprovedAt      *time.Time           `gorm:"type:timestamptz"`
	AppliedAt       *time.Time           `gorm:"type:timestamptz"`
	CreatedBy       uuid.UUID            `gorm:"type:uuid;not null"`
	CreatedAt       time.Time            `gorm:"type:timestamptz;not null;index:ix_stb_batches_episode_created,priority:2,sort:desc"`
	UpdatedAt       time.Time            `gorm:"type:timestamptz;not null"`
	Workspace       Workspace            `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project         Project              `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Episode         Episode              `gorm:"foreignKey:EpisodeID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Structure       EpisodeStructure     `gorm:"foreignKey:StructureID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	ScriptVersion   EpisodeScriptVersion `gorm:"foreignKey:ScriptVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Task            WorkflowTask         `gorm:"foreignKey:TaskID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator         UserAccount          `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Approver        *UserAccount         `gorm:"foreignKey:ApprovedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (StoryboardDraftBatch) TableName() string { return "stb_draft_batches" }

type StoryboardShot struct {
	ID               uuid.UUID            `gorm:"type:uuid;primaryKey"`
	WorkspaceID      uuid.UUID            `gorm:"type:uuid;not null"`
	ProjectID        uuid.UUID            `gorm:"type:uuid;not null"`
	EpisodeID        uuid.UUID            `gorm:"type:uuid;not null;index:ix_stb_shots_episode_status,priority:1"`
	BatchID          uuid.UUID            `gorm:"type:uuid;not null;uniqueIndex:uq_stb_shot_batch_position,priority:1"`
	ProposalKey      string               `gorm:"type:varchar(120);not null"`
	Position         int                  `gorm:"not null;uniqueIndex:uq_stb_shot_batch_position,priority:2;check:ck_stb_shot_position,position >= 1"`
	Title            string               `gorm:"type:varchar(200);not null"`
	NarrativeUnitIDs datatypes.JSON       `gorm:"type:jsonb;not null"`
	Spec             datatypes.JSON       `gorm:"type:jsonb;not null"`
	ContentHash      string               `gorm:"type:char(64);not null;check:ck_stb_shot_hash,char_length(content_hash) = 64"`
	Status           string               `gorm:"type:varchar(20);not null;index:ix_stb_shots_episode_status,priority:2;check:ck_stb_shot_status,status IN ('active','archived')"`
	Revision         int                  `gorm:"not null;check:ck_stb_shot_revision,revision >= 1"`
	CreatedBy        uuid.UUID            `gorm:"type:uuid;not null"`
	CreatedAt        time.Time            `gorm:"type:timestamptz;not null"`
	UpdatedAt        time.Time            `gorm:"type:timestamptz;not null"`
	Workspace        Workspace            `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project          Project              `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Episode          Episode              `gorm:"foreignKey:EpisodeID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Batch            StoryboardDraftBatch `gorm:"foreignKey:BatchID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator          UserAccount          `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (StoryboardShot) TableName() string { return "stb_shots" }

type StoryboardExport struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID uuid.UUID      `gorm:"type:uuid;not null"`
	ProjectID   uuid.UUID      `gorm:"type:uuid;not null"`
	EpisodeID   uuid.UUID      `gorm:"type:uuid;not null;index:ix_stb_exports_episode_created,priority:1"`
	Status      string         `gorm:"type:varchar(20);not null;check:ck_stb_export_status,status IN ('succeeded','failed')"`
	InputHash   string         `gorm:"type:char(64);not null;check:ck_stb_export_input_hash,char_length(input_hash) = 64"`
	ContentHash string         `gorm:"type:char(64);not null;check:ck_stb_export_content_hash,char_length(content_hash) = 64"`
	Manifest    datatypes.JSON `gorm:"type:jsonb;not null"`
	Files       datatypes.JSON `gorm:"type:jsonb;not null"`
	Package     []byte         `gorm:"type:bytea;not null"`
	Revision    int            `gorm:"not null;check:ck_stb_export_revision,revision >= 1"`
	CreatedBy   uuid.UUID      `gorm:"type:uuid;not null"`
	CreatedAt   time.Time      `gorm:"type:timestamptz;not null;index:ix_stb_exports_episode_created,priority:2,sort:desc"`
	UpdatedAt   time.Time      `gorm:"type:timestamptz;not null"`
	Workspace   Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project     Project        `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Episode     Episode        `gorm:"foreignKey:EpisodeID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Creator     UserAccount    `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (StoryboardExport) TableName() string { return "stb_exports" }
