package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

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
