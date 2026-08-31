package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type SourceSpanIndexVersion struct {
	ID                   uuid.UUID        `gorm:"type:uuid;primaryKey"`
	WorkspaceID          uuid.UUID        `gorm:"type:uuid;not null;index:ix_scr_source_index_workspace_created,priority:1"`
	ProjectID            uuid.UUID        `gorm:"type:uuid;not null;index:ix_scr_source_index_project_created,priority:1"`
	DocumentRevisionID   uuid.UUID        `gorm:"type:uuid;not null;uniqueIndex:uq_scr_source_index_revision"`
	SourceHash           string           `gorm:"type:char(64);not null;check:ck_scr_source_index_source_hash,char_length(source_hash) = 64"`
	NewlineNormalization string           `gorm:"type:varchar(20);not null;check:ck_scr_source_index_newline,newline_normalization = 'lf'"`
	CodepointIndexRule   string           `gorm:"type:varchar(40);not null;check:ck_scr_source_index_rule,codepoint_index_rule = 'unicode-code-point'"`
	CodepointCount       int              `gorm:"not null;check:ck_scr_source_index_codepoints,codepoint_count >= 1"`
	UTF8ByteCount        int              `gorm:"not null;check:ck_scr_source_index_bytes,utf8_byte_count >= codepoint_count"`
	IndexManifest        datatypes.JSON   `gorm:"type:jsonb;not null;check:ck_scr_source_index_manifest,jsonb_typeof(index_manifest) = 'object'"`
	ContentHash          string           `gorm:"type:char(64);not null;uniqueIndex;check:ck_scr_source_index_content_hash,char_length(content_hash) = 64"`
	CreatedBy            uuid.UUID        `gorm:"type:uuid;not null"`
	CreatedAt            time.Time        `gorm:"type:timestamptz;not null;index:ix_scr_source_index_workspace_created,priority:2,sort:desc;index:ix_scr_source_index_project_created,priority:2,sort:desc"`
	Workspace            Workspace        `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project              Project          `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	DocumentRevision     DocumentRevision `gorm:"foreignKey:DocumentRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator              UserAccount      `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (SourceSpanIndexVersion) TableName() string { return "scr_source_span_indexes" }

type ScriptSourceScopeHead struct {
	ProjectID                 uuid.UUID              `gorm:"type:uuid;primaryKey"`
	WorkspaceID               uuid.UUID              `gorm:"type:uuid;not null;index:ix_scr_source_head_workspace"`
	DocumentLogicalID         uuid.UUID              `gorm:"type:uuid;not null"`
	CurrentDocumentRevisionID uuid.UUID              `gorm:"type:uuid;not null;uniqueIndex:uq_scr_source_head_revision"`
	CurrentSpanIndexID        uuid.UUID              `gorm:"type:uuid;not null;uniqueIndex:uq_scr_source_head_index"`
	HeadRevision              int64                  `gorm:"not null;check:ck_scr_source_head_revision,head_revision >= 1"`
	HeadHash                  string                 `gorm:"type:char(64);not null;check:ck_scr_source_head_hash,char_length(head_hash) = 64"`
	UpdatedAt                 time.Time              `gorm:"type:timestamptz;not null"`
	Workspace                 Workspace              `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                   Project                `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CurrentDocumentRevision   DocumentRevision       `gorm:"foreignKey:CurrentDocumentRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CurrentSpanIndex          SourceSpanIndexVersion `gorm:"foreignKey:CurrentSpanIndexID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ScriptSourceScopeHead) TableName() string { return "scr_source_scope_heads" }

type ScriptSourceCollectionReceipt struct {
	ID                  uuid.UUID              `gorm:"type:uuid;primaryKey"`
	WorkspaceID         uuid.UUID              `gorm:"type:uuid;not null;uniqueIndex:uq_scr_source_collection_receipt,priority:1"`
	ProjectID           uuid.UUID              `gorm:"type:uuid;not null;uniqueIndex:uq_scr_source_collection_receipt,priority:2;index:ix_scr_source_collection_project_created,priority:1"`
	DocumentRevisionID  uuid.UUID              `gorm:"type:uuid;not null;uniqueIndex:uq_scr_source_collection_receipt,priority:3"`
	SpanIndexID         uuid.UUID              `gorm:"type:uuid;not null;uniqueIndex:uq_scr_source_collection_receipt,priority:4"`
	HeadRevision        int64                  `gorm:"not null;check:ck_scr_source_collection_head_revision,head_revision >= 1"`
	HeadHash            string                 `gorm:"type:char(64);not null;check:ck_scr_source_collection_head_hash,char_length(head_hash) = 64"`
	Members             datatypes.JSON         `gorm:"type:jsonb;not null;check:ck_scr_source_collection_members,jsonb_typeof(members) = 'array'"`
	MembersHash         string                 `gorm:"type:char(64);not null;check:ck_scr_source_collection_members_hash,char_length(members_hash) = 64"`
	CollectionRootHash  string                 `gorm:"type:char(64);not null;uniqueIndex:uq_scr_source_collection_receipt,priority:5;check:ck_scr_source_collection_root_hash,char_length(collection_root_hash) = 64"`
	SourceAcceptanceRef string                 `gorm:"type:varchar(200);not null"`
	ReceiptContentHash  string                 `gorm:"type:char(64);not null;uniqueIndex;check:ck_scr_source_collection_receipt_hash,char_length(receipt_content_hash) = 64"`
	CreatedBy           uuid.UUID              `gorm:"type:uuid;not null"`
	CreatedAt           time.Time              `gorm:"type:timestamptz;not null;index:ix_scr_source_collection_project_created,priority:2,sort:desc"`
	Workspace           Workspace              `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project             Project                `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	DocumentRevision    DocumentRevision       `gorm:"foreignKey:DocumentRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	SpanIndex           SourceSpanIndexVersion `gorm:"foreignKey:SpanIndexID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator             UserAccount            `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ScriptSourceCollectionReceipt) TableName() string {
	return "scr_source_collection_receipts"
}
