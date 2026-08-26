package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var ErrImmutableStoryGraphVersion = errors.New("published StoryGraphVersion is immutable")

type StoryGraphVersion struct {
	ID                 uuid.UUID          `gorm:"type:uuid;primaryKey"`
	WorkspaceID        uuid.UUID          `gorm:"type:uuid;not null"`
	ProjectID          uuid.UUID          `gorm:"type:uuid;not null;uniqueIndex:uq_sg_version_project_no,priority:1;index:ix_sg_versions_project_published,priority:1"`
	VersionNo          int64              `gorm:"not null;uniqueIndex:uq_sg_version_project_no,priority:2;check:ck_sg_version_no,version_no >= 1"`
	ParentVersionID    *uuid.UUID         `gorm:"type:uuid;check:ck_sg_parent_chain,(version_no = 1 AND parent_version_id IS NULL AND parent_content_hash IS NULL) OR (version_no > 1 AND parent_version_id IS NOT NULL AND parent_content_hash IS NOT NULL)"`
	ParentContentHash  *string            `gorm:"type:char(64);check:ck_sg_parent_hash,parent_content_hash IS NULL OR char_length(parent_content_hash) = 64"`
	SourceRevisionID   uuid.UUID          `gorm:"type:uuid;not null"`
	SourceRevisionHash string             `gorm:"type:char(64);not null;check:ck_sg_source_hash,char_length(source_revision_hash) = 64"`
	OwnerHeadRefs      datatypes.JSON     `gorm:"type:jsonb;not null;check:ck_sg_owner_heads,jsonb_typeof(owner_head_refs) = 'array'"`
	OwnerSetHash       string             `gorm:"type:char(64);not null;check:ck_sg_owner_set_hash,char_length(owner_set_hash) = 64"`
	SchemaVersion      string             `gorm:"type:varchar(40);not null"`
	Nodes              datatypes.JSON     `gorm:"type:jsonb;not null;check:ck_sg_nodes,jsonb_typeof(nodes) = 'array'"`
	Edges              datatypes.JSON     `gorm:"type:jsonb;not null;check:ck_sg_edges,jsonb_typeof(edges) = 'array'"`
	TopologyHash       string             `gorm:"type:char(64);not null;check:ck_sg_topology_hash,char_length(topology_hash) = 64"`
	ContentHash        string             `gorm:"type:char(64);not null;check:ck_sg_content_hash,char_length(content_hash) = 64"`
	Status             string             `gorm:"type:varchar(20);not null;check:ck_sg_version_status,status = 'published'"`
	PublishedAt        time.Time          `gorm:"type:timestamptz;not null;index:ix_sg_versions_project_published,priority:2,sort:desc"`
	CreatedBy          uuid.UUID          `gorm:"type:uuid;not null"`
	CreatedAt          time.Time          `gorm:"type:timestamptz;not null"`
	Workspace          Workspace          `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project            Project            `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	ParentVersion      *StoryGraphVersion `gorm:"foreignKey:ParentVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	SourceRevision     DocumentRevision   `gorm:"foreignKey:SourceRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator            UserAccount        `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (StoryGraphVersion) TableName() string { return "sg_storygraph_versions" }

func (*StoryGraphVersion) BeforeUpdate(*gorm.DB) error { return ErrImmutableStoryGraphVersion }
func (*StoryGraphVersion) BeforeDelete(*gorm.DB) error { return ErrImmutableStoryGraphVersion }

type StoryGraphHead struct {
	WorkspaceID        uuid.UUID         `gorm:"type:uuid;not null"`
	ProjectID          uuid.UUID         `gorm:"type:uuid;primaryKey"`
	CurrentVersionID   uuid.UUID         `gorm:"type:uuid;not null;uniqueIndex"`
	CurrentContentHash string            `gorm:"type:char(64);not null;check:ck_sg_head_hash,char_length(current_content_hash) = 64"`
	Revision           int64             `gorm:"not null;check:ck_sg_head_revision,revision >= 1"`
	UpdatedAt          time.Time         `gorm:"type:timestamptz;not null"`
	Workspace          Workspace         `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project            Project           `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CurrentVersion     StoryGraphVersion `gorm:"foreignKey:CurrentVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (StoryGraphHead) TableName() string { return "sg_storygraph_heads" }
