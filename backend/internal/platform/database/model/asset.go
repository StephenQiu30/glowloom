package model

import (
	"time"

	"github.com/google/uuid"
)

type Artifact struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	WorkspaceID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_ast_artifact_source_output,priority:1;index:ix_ast_artifact_workspace_status,priority:1"`
	ProjectID   uuid.UUID `gorm:"type:uuid;not null;index"`
	SourceType  string    `gorm:"type:varchar(40);not null;uniqueIndex:uq_ast_artifact_source_output,priority:2;check:ck_ast_artifact_source_type,source_type IN ('generation_provider_job','media_job','upload')"`
	SourceID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_ast_artifact_source_output,priority:3"`
	OutputKey   string    `gorm:"type:varchar(120);not null;uniqueIndex:uq_ast_artifact_source_output,priority:4"`
	MediaType   string    `gorm:"type:varchar(120);not null;check:ck_ast_artifact_media_type,media_type IN ('image/png','image/jpeg')"`
	SHA256      string    `gorm:"type:char(64);not null;check:ck_ast_artifact_sha256,char_length(sha256) = 64"`
	SizeBytes   int64     `gorm:"not null;check:ck_ast_artifact_size,size_bytes > 0"`
	Status      string    `gorm:"type:varchar(24);not null;index:ix_ast_artifact_workspace_status,priority:2;check:ck_ast_artifact_status,status IN ('PENDING_VALIDATION','READY','QUARANTINED','UNAVAILABLE','TOMBSTONED')"`
	FailureCode *string   `gorm:"type:varchar(80)"`
	Width       *int      `gorm:"check:ck_ast_artifact_width,width IS NULL OR width > 0"`
	Height      *int      `gorm:"check:ck_ast_artifact_height,height IS NULL OR height > 0"`
	Revision    int       `gorm:"not null;check:ck_ast_artifact_revision,revision >= 1"`
	CreatedAt   time.Time `gorm:"type:timestamptz;not null"`
	UpdatedAt   time.Time `gorm:"type:timestamptz;not null"`
	Workspace   Workspace `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project     Project   `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (Artifact) TableName() string { return "ast_artifacts" }

type ArtifactLocation struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	WorkspaceID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_ast_location_object,priority:1"`
	ArtifactID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_ast_location_no,priority:1"`
	LocationNo     int       `gorm:"not null;uniqueIndex:uq_ast_location_no,priority:2;check:ck_ast_location_no,location_no >= 1"`
	StorageProfile string    `gorm:"type:varchar(80);not null;uniqueIndex:uq_ast_location_object,priority:2"`
	Bucket         string    `gorm:"type:varchar(120);not null;uniqueIndex:uq_ast_location_object,priority:3"`
	ObjectKey      string    `gorm:"type:varchar(600);not null;uniqueIndex:uq_ast_location_object,priority:4"`
	Region         string    `gorm:"type:varchar(80);not null"`
	Checksum       string    `gorm:"type:char(64);not null;check:ck_ast_location_checksum,char_length(checksum) = 64"`
	Status         string    `gorm:"type:varchar(20);not null;check:ck_ast_location_status,status IN ('STAGING','PRIMARY','SECONDARY','RETIRED','UNAVAILABLE')"`
	CreatedAt      time.Time `gorm:"type:timestamptz;not null"`
	UpdatedAt      time.Time `gorm:"type:timestamptz;not null"`
	Workspace      Workspace `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Artifact       Artifact  `gorm:"foreignKey:ArtifactID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

func (ArtifactLocation) TableName() string { return "ast_artifact_locations" }
