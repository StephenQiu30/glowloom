package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type ScriptDocument struct {
	ID                   uuid.UUID     `gorm:"type:uuid;primaryKey"`
	WorkspaceID          uuid.UUID     `gorm:"type:uuid;not null;index:ix_scr_documents_workspace_project_created,priority:1"`
	ProjectID            uuid.UUID     `gorm:"type:uuid;not null;index:ix_scr_documents_workspace_project_created,priority:2"`
	Title                string        `gorm:"type:varchar(120);not null"`
	SourceType           string        `gorm:"type:varchar(20);not null;check:ck_scr_document_source_type,source_type IN ('text','media')"`
	SourceMediaVersionID *uuid.UUID    `gorm:"type:uuid"`
	Language             string        `gorm:"type:varchar(35);not null"`
	RightsDeclaration    string        `gorm:"type:varchar(1000);not null"`
	Status               string        `gorm:"type:varchar(20);not null;check:ck_scr_document_status,status IN ('active','archived')"`
	Revision             int           `gorm:"not null;check:ck_scr_document_revision,revision >= 1"`
	CreatedBy            uuid.UUID     `gorm:"type:uuid;not null"`
	CreatedAt            time.Time     `gorm:"type:timestamptz;not null;index:ix_scr_documents_workspace_project_created,priority:3,sort:desc"`
	UpdatedAt            time.Time     `gorm:"type:timestamptz;not null"`
	Workspace            Workspace     `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project              Project       `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	SourceMediaVersion   *MediaVersion `gorm:"foreignKey:SourceMediaVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator              UserAccount   `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ScriptDocument) TableName() string { return "scr_documents" }

type DocumentRevision struct {
	ID                   uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID          uuid.UUID      `gorm:"type:uuid;not null;index:ix_scr_revisions_workspace_created,priority:1"`
	DocumentID           uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_scr_revision_document_version,priority:1"`
	VersionNo            int            `gorm:"not null;uniqueIndex:uq_scr_revision_document_version,priority:2;check:ck_scr_revision_version,version_no >= 1"`
	SourceType           string         `gorm:"type:varchar(20);not null"`
	SourceMediaVersionID *uuid.UUID     `gorm:"type:uuid"`
	RawText              string         `gorm:"type:text;not null"`
	RawHash              string         `gorm:"type:char(64);not null;check:ck_scr_revision_raw_hash,char_length(raw_hash) = 64"`
	NormalizedText       string         `gorm:"type:text;not null"`
	NormalizedHash       string         `gorm:"type:char(64);not null;check:ck_scr_revision_normalized_hash,char_length(normalized_hash) = 64"`
	NormalizerVersion    string         `gorm:"type:varchar(40);not null"`
	NormalizationMap     datatypes.JSON `gorm:"type:jsonb;not null"`
	CodepointCount       int            `gorm:"not null;check:ck_scr_revision_codepoints,codepoint_count >= 0"`
	AnalysisStatus       string         `gorm:"type:varchar(30);not null;check:ck_scr_revision_analysis_status,analysis_status IN ('deterministic','ai_candidate_required','rejected')"`
	AnalyzerVersion      string         `gorm:"type:varchar(40);not null"`
	Blocks               datatypes.JSON `gorm:"type:jsonb;not null"`
	Issues               datatypes.JSON `gorm:"type:jsonb;not null"`
	CreatedBy            uuid.UUID      `gorm:"type:uuid;not null"`
	CreatedAt            time.Time      `gorm:"type:timestamptz;not null;index:ix_scr_revisions_workspace_created,priority:2,sort:desc"`
	Workspace            Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Document             ScriptDocument `gorm:"foreignKey:DocumentID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	SourceMediaVersion   *MediaVersion  `gorm:"foreignKey:SourceMediaVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator              UserAccount    `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (DocumentRevision) TableName() string { return "scr_document_revisions" }
