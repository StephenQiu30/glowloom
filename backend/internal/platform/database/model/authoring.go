package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type NodeDefinitionVersion struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey"`
	Key          string         `gorm:"type:varchar(100);not null;uniqueIndex:uq_aut_node_definition_key_version,priority:1"`
	Version      string         `gorm:"type:varchar(40);not null;uniqueIndex:uq_aut_node_definition_key_version,priority:2"`
	Name         string         `gorm:"type:varchar(120);not null"`
	Category     string         `gorm:"type:varchar(40);not null"`
	Executor     string         `gorm:"type:varchar(120);not null"`
	InputPorts   datatypes.JSON `gorm:"type:jsonb;not null"`
	OutputPorts  datatypes.JSON `gorm:"type:jsonb;not null"`
	ConfigSchema datatypes.JSON `gorm:"type:jsonb;not null"`
	CachePolicy  string         `gorm:"type:varchar(20);not null;check:ck_aut_node_cache_policy,cache_policy IN ('never','by_inputs')"`
	RiskLevel    string         `gorm:"type:varchar(20);not null;check:ck_aut_node_risk_level,risk_level IN ('low','external_ai','human_gate')"`
	Executable   bool           `gorm:"not null"`
	ContentHash  string         `gorm:"type:char(64);not null;check:ck_aut_node_content_hash,char_length(content_hash) = 64"`
	CreatedAt    time.Time      `gorm:"type:timestamptz;not null"`
}

func (NodeDefinitionVersion) TableName() string { return "aut_node_definition_versions" }

type NodeCatalogVersion struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey"`
	Key           string         `gorm:"type:varchar(100);not null;uniqueIndex:uq_aut_node_catalog_key_version,priority:1"`
	Version       string         `gorm:"type:varchar(40);not null;uniqueIndex:uq_aut_node_catalog_key_version,priority:2"`
	Definitions   datatypes.JSON `gorm:"type:jsonb;not null"`
	ContentHash   string         `gorm:"type:char(64);not null;check:ck_aut_catalog_content_hash,char_length(content_hash) = 64"`
	ExecutionHash string         `gorm:"type:char(64);not null;check:ck_aut_catalog_execution_hash,char_length(execution_hash) = 64"`
	Status        string         `gorm:"type:varchar(20);not null;check:ck_aut_catalog_status,status = 'published'"`
	CreatedAt     time.Time      `gorm:"type:timestamptz;not null"`
}

func (NodeCatalogVersion) TableName() string { return "aut_node_catalog_versions" }

type AuthoringDraft struct {
	ID                         uuid.UUID          `gorm:"type:uuid;primaryKey"`
	WorkspaceID                uuid.UUID          `gorm:"type:uuid;not null;index:ix_aut_drafts_workspace_updated,priority:1"`
	ProjectID                  uuid.UUID          `gorm:"type:uuid;not null;index:ix_aut_drafts_project_status_updated,priority:1"`
	AuthoringMode              string             `gorm:"type:varchar(20);not null;check:ck_aut_draft_mode,authoring_mode IN ('GUIDED','CANVAS')"`
	Graph                      datatypes.JSON     `gorm:"type:jsonb;not null"`
	Layout                     datatypes.JSON     `gorm:"type:jsonb;not null"`
	FrozenInputs               datatypes.JSON     `gorm:"type:jsonb;not null"`
	NodeCatalogVersionID       uuid.UUID          `gorm:"type:uuid;not null"`
	Status                     string             `gorm:"type:varchar(20);not null;index:ix_aut_drafts_project_status_updated,priority:2;check:ck_aut_draft_status,status IN ('active','archived')"`
	Revision                   int                `gorm:"not null;check:ck_aut_draft_revision,revision >= 1"`
	CurrentPublishedRevisionID *uuid.UUID         `gorm:"type:uuid"`
	CreatedBy                  uuid.UUID          `gorm:"type:uuid;not null"`
	CreatedAt                  time.Time          `gorm:"type:timestamptz;not null"`
	UpdatedAt                  time.Time          `gorm:"type:timestamptz;not null;index:ix_aut_drafts_workspace_updated,priority:2,sort:desc;index:ix_aut_drafts_project_status_updated,priority:3,sort:desc"`
	Workspace                  Workspace          `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                    Project            `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	NodeCatalogVersion         NodeCatalogVersion `gorm:"foreignKey:NodeCatalogVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator                    UserAccount        `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (AuthoringDraft) TableName() string { return "aut_drafts" }

type AuthoringRevision struct {
	ID                   uuid.UUID          `gorm:"type:uuid;primaryKey"`
	WorkspaceID          uuid.UUID          `gorm:"type:uuid;not null;index:ix_aut_revisions_workspace_created,priority:1"`
	ProjectID            uuid.UUID          `gorm:"type:uuid;not null;index:ix_aut_revisions_project_created,priority:1"`
	DraftID              uuid.UUID          `gorm:"type:uuid;not null;uniqueIndex:uq_aut_revision_draft_no,priority:1"`
	RevisionNo           int                `gorm:"not null;uniqueIndex:uq_aut_revision_draft_no,priority:2;check:ck_aut_revision_no,revision_no >= 1"`
	AuthoringMode        string             `gorm:"type:varchar(20);not null;check:ck_aut_revision_mode,authoring_mode IN ('GUIDED','CANVAS')"`
	Graph                datatypes.JSON     `gorm:"type:jsonb;not null"`
	Layout               datatypes.JSON     `gorm:"type:jsonb;not null"`
	FrozenInputs         datatypes.JSON     `gorm:"type:jsonb;not null"`
	NodeCatalogVersionID uuid.UUID          `gorm:"type:uuid;not null"`
	CatalogContentHash   string             `gorm:"type:char(64);not null;check:ck_aut_revision_catalog_hash,char_length(catalog_content_hash) = 64"`
	CatalogExecutionHash string             `gorm:"type:char(64);not null;check:ck_aut_revision_catalog_execution_hash,char_length(catalog_execution_hash) = 64"`
	ExecutionHash        string             `gorm:"type:char(64);not null;index:ix_aut_revisions_execution_hash;check:ck_aut_revision_execution_hash,char_length(execution_hash) = 64"`
	ContentHash          string             `gorm:"type:char(64);not null;index:ix_aut_revisions_content_hash;check:ck_aut_revision_content_hash,char_length(content_hash) = 64"`
	CreatedBy            uuid.UUID          `gorm:"type:uuid;not null"`
	CreatedAt            time.Time          `gorm:"type:timestamptz;not null;index:ix_aut_revisions_workspace_created,priority:2,sort:desc;index:ix_aut_revisions_project_created,priority:2,sort:desc"`
	Workspace            Workspace          `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project              Project            `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Draft                AuthoringDraft     `gorm:"foreignKey:DraftID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	NodeCatalogVersion   NodeCatalogVersion `gorm:"foreignKey:NodeCatalogVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator              UserAccount        `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (AuthoringRevision) TableName() string { return "aut_revisions" }
